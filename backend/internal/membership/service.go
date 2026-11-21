package membership

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"giki-sports-backend/internal/wallet"
)

// Service coordinates 30-day gym pass validity, double-entry wallet payments, and auditing.
type Service struct {
	pool      *pgxpool.Pool
	repo      Repository
	walletSvc *wallet.Service
	clock     Clock
}

// NewService constructs a new membership Service instance.
func NewService(pool *pgxpool.Pool, repo Repository, walletSvc *wallet.Service, clock Clock) *Service {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{
		pool:      pool,
		repo:      repo,
		walletSvc: walletSvc,
		clock:     clock,
	}
}

// PurchaseOrRenewPass handles atomic pass purchasing or renewal (FR-04, QR-04, QR-06).
// Invariants:
// 1. Double-entry wallet debit: debits user personal wallet, credits SYS_MEMBERSHIP_REVENUE (FR-04).
// 2. Insufficient funds causes immediate rollback with ErrInsufficientFunds; 0 passes created.
// 3. Strict 30-day term [T_start, T_start + 30 days) (QR-06).
// 4. Renewal adds 30 days to max(T_existing_expiry, T_renewal) (QR-06).
// 5. User-scoped idempotency replay returns the existing pass without duplicate debits (QR-04).
func (s *Service) PurchaseOrRenewPass(ctx context.Context, userID uuid.UUID, planType PlanType, idempotencyKey string) (*Membership, error) {
	if userID == uuid.Nil {
		return nil, errors.New("user ID is required")
	}
	if planType != PlanTypeStandard30Day {
		return nil, ErrInvalidPlanType
	}
	trimmedKey := strings.TrimSpace(idempotencyKey)
	if trimmedKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Idempotency replay check
	existing, err := s.repo.GetMembershipByIdempotencyTx(ctx, tx, userID, trimmedKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check error: %w", err)
	}
	if existing != nil {
		_ = tx.Commit(ctx)
		return existing, nil
	}

	// 2. Resolve user personal wallet
	userWalletID, err := s.walletSvc.FindPersonalWalletTx(ctx, tx, userID)
	if err != nil {
		if errors.Is(err, wallet.ErrWalletNotFound) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("failed to locate personal wallet: %w", err)
	}

	// 3. Resolve system revenue wallet
	revenueWalletID, err := s.walletSvc.FindOrCreateSystemWalletTx(ctx, tx, "SYS_MEMBERSHIP_REVENUE", wallet.WalletTypeRevenue, "PKR")
	if err != nil {
		return nil, fmt.Errorf("failed to locate revenue wallet: %w", err)
	}

	// 4. Query user's current latest active or upcoming pass (with row-level lock)
	now := s.clock.Now().UTC()
	latest, err := s.repo.GetLatestActiveOrUpcomingMembershipTx(ctx, tx, userID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query active pass: %w", err)
	}

	var existingExpiry *time.Time
	if latest != nil {
		exp := latest.EndTime
		existingExpiry = &exp
	}

	// 5. Query active plan for dynamic fee and duration
	var plan *GymMembershipPlan
	if planType != "" {
		plan, _ = s.repo.GetActivePlanTx(ctx, tx, string(planType))
	}
	if plan == nil {
		plan, _ = s.repo.GetDefaultPlanTx(ctx, tx)
		if plan == nil {
			plan = &GymMembershipPlan{
				PlanCode:     string(PlanTypeStandard30Day),
				DurationDays: 30,
				PricePaisa:   DefaultGymMembershipFeePaisa,
			}
		}
	}

	planDuration := time.Duration(plan.DurationDays) * 24 * time.Hour
	startTime, endTime := CalculateRenewalTermWithDuration(existingExpiry, now, planDuration)

	// 6. Execute atomic wallet debit
	fee := plan.PricePaisa
	txParams := wallet.TransferParams{
		SenderWalletID:   userWalletID,
		ReceiverWalletID: revenueWalletID,
		Amount:           fee,
		Type:             "GYM_MEMBERSHIP",
		ReferenceID:      fmt.Sprintf("gym-%s-%s", userID.String()[:8], trimmedKey),
		Description:      fmt.Sprintf("%d-Day Gym Membership Pass", plan.DurationDays),
	}

	postResult, err := s.walletSvc.PostTransactionTx(ctx, tx, txParams)
	if err != nil {
		if errors.Is(err, wallet.ErrInsufficientFunds) {
			return nil, ErrInsufficientFunds
		}
		return nil, fmt.Errorf("wallet debit error: %w", err)
	}

	// 7. Insert gym membership pass
	mbr := &Membership{
		ID:                 uuid.New(),
		UserID:             userID,
		PlanType:           planType,
		Status:             MembershipStatusActive,
		StartTime:          startTime,
		EndTime:            endTime,
		PricePaisa:         fee,
		DebitTransactionID: postResult.TransactionID,
		IdempotencyKey:     trimmedKey,
	}

	if err := s.repo.CreateMembershipTx(ctx, tx, mbr); err != nil {
		if errors.Is(err, ErrDuplicatePurchase) {
			return nil, ErrDuplicatePurchase
		}
		return nil, fmt.Errorf("failed to persist membership: %w", err)
	}

	// 8. Record audit log
	auditDetails := map[string]interface{}{
		"plan_type":        string(planType),
		"fee_paisa":        fee,
		"debit_tx_id":      postResult.TransactionID.String(),
		"idempotency_key":  trimmedKey,
		"start_time":       startTime.Format(time.RFC3339),
		"end_time":         endTime.Format(time.RFC3339),
		"is_early_renewal": existingExpiry != nil && existingExpiry.After(now),
	}
	_ = s.repo.RecordAuditLogTx(ctx, tx, userID, "membership.purchase", "gym_membership", mbr.ID, auditDetails)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit membership purchase: %w", err)
	}

	return mbr, nil
}

// GetUserSummary aggregates the user's membership pass history, active status, and remaining time.
func (s *Service) GetUserSummary(ctx context.Context, userID uuid.UUID) (*UserMembershipSummary, error) {
	if userID == uuid.Nil {
		return nil, errors.New("user ID is required")
	}

	history, err := s.repo.ListMembershipsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user memberships: %w", err)
	}

	summary := ComputeMembershipSummary(history, s.clock.Now())
	return &summary, nil
}

// ListAllMemberships returns a paginated list of memberships for staff/attendant review.
func (s *Service) ListAllMemberships(ctx context.Context, limit, offset int) ([]*Membership, error) {
	return s.repo.ListAllMemberships(ctx, limit, offset)
}

// GetMembershipByID retrieves an individual membership pass.
func (s *Service) GetMembershipByID(ctx context.Context, id uuid.UUID) (*Membership, error) {
	return s.repo.GetMembershipByID(ctx, id)
}

// GetDefaultPlan returns the primary active gym membership plan.
func (s *Service) GetDefaultPlan(ctx context.Context) (*GymMembershipPlan, error) {
	plan, err := s.repo.GetDefaultPlan(ctx)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			return &GymMembershipPlan{
				PlanCode:     string(PlanTypeStandard30Day),
				Name:         "Standard Gym Membership",
				DurationDays: 30,
				PricePaisa:   DefaultGymMembershipFeePaisa,
				IsActive:     true,
				Description:  "Olympic racks, specialized free weights, cardio machines, and functional rigs.",
			}, nil
		}
		return nil, err
	}
	return plan, nil
}

// UpdatePlan allows sports directors to modify pass name, duration, price, and description.
func (s *Service) UpdatePlan(ctx context.Context, planCode string, name string, durationDays int, pricePaisa int64, description string, directorID uuid.UUID) (*GymMembershipPlan, error) {
	if durationDays <= 0 {
		return nil, ErrInvalidPlanDuration
	}
	if pricePaisa < 0 {
		return nil, ErrNegativePlanFee
	}
	if strings.TrimSpace(planCode) == "" {
		planCode = string(PlanTypeStandard30Day)
	}

	updated, err := s.repo.UpdatePlan(ctx, planCode, strings.TrimSpace(name), durationDays, pricePaisa, strings.TrimSpace(description), directorID)
	if err != nil {
		return nil, err
	}

	auditDetails := map[string]interface{}{
		"plan_code":     planCode,
		"name":          name,
		"duration_days": durationDays,
		"price_paisa":   pricePaisa,
		"description":   description,
	}
	_ = s.repo.RecordAuditLogTx(ctx, nil, directorID, "membership.plan_update", "gym_membership_plan", updated.ID, auditDetails)

	return updated, nil
}
