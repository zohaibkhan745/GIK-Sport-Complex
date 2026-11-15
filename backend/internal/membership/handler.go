package membership

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
)

// PurchasePassRequest represents the incoming JSON payload to purchase or renew a pass.
type PurchasePassRequest struct {
	PlanType       string `json:"planType"`
	Plan           string `json:"plan"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// MembershipResponse serializes a membership domain entity for client consumers.
type MembershipResponse struct {
	ID                 string `json:"id"`
	UserID             string `json:"userId"`
	PlanType           string `json:"planType"`
	Status             string `json:"status"`
	StartTime          string `json:"startTime"`
	EndTime            string `json:"endTime"`
	PricePaisa         int64  `json:"pricePaisa"`
	DebitTransactionID string `json:"debitTransactionId"`
	IdempotencyKey     string `json:"idempotencyKey"`
	CreatedAt          string `json:"createdAt"`
}

// SummaryResponse formats a user's active membership status, countdown, and history.
type SummaryResponse struct {
	IsActive          bool                  `json:"isActive"`
	CurrentMembership *MembershipResponse   `json:"currentMembership,omitempty"`
	EffectiveExpiry   *string               `json:"effectiveExpiry,omitempty"`
	DaysRemaining     int                   `json:"daysRemaining"`
	SecondsRemaining  int64                 `json:"secondsRemaining"`
	History           []*MembershipResponse `json:"history"`
}

// Handler provides HTTP endpoints for gym membership operations.
type Handler struct {
	service      *Service
	queryTimeout time.Duration
}

// NewHandler constructs a new Handler.
func NewHandler(service *Service, queryTimeout time.Duration) *Handler {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	return &Handler{
		service:      service,
		queryTimeout: queryTimeout,
	}
}

// RegisterRoutes mounts all authenticated and staff routes for gym memberships.
func (h *Handler) RegisterRoutes(
	router fiber.Router,
	requireOrigin fiber.Handler,
	requireAuth fiber.Handler,
	requireStaff fiber.Handler,
) {
	// Authenticated student/faculty/staff endpoints
	router.Get("/memberships/me", requireOrigin, requireAuth, h.GetMySummary)
	router.Post("/memberships/purchase", requireOrigin, requireAuth, h.PurchasePass)
	router.Post("/memberships", requireOrigin, requireAuth, h.PurchasePass)

	// Administrative staff endpoints
	router.Get("/admin/memberships", requireOrigin, requireAuth, requireStaff, h.ListMembershipsAdmin)
}

func toResponse(m *Membership) *MembershipResponse {
	if m == nil {
		return nil
	}
	return &MembershipResponse{
		ID:                 m.ID.String(),
		UserID:             m.UserID.String(),
		PlanType:           string(m.PlanType),
		Status:             string(m.Status),
		StartTime:          m.StartTime.UTC().Format(time.RFC3339),
		EndTime:            m.EndTime.UTC().Format(time.RFC3339),
		PricePaisa:         m.PricePaisa,
		DebitTransactionID: m.DebitTransactionID.String(),
		IdempotencyKey:     m.IdempotencyKey,
		CreatedAt:          m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// GetMySummary handles GET /api/memberships/me.
func (h *Handler) GetMySummary(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized: authentication required",
		})
	}

	ctx, cancel := context.WithTimeout(c.Context(), h.queryTimeout)
	defer cancel()

	summary, err := h.service.GetUserSummary(ctx, principal.User.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to retrieve membership summary",
		})
	}

	var historyResp []*MembershipResponse
	for _, m := range summary.History {
		historyResp = append(historyResp, toResponse(m))
	}

	var expStr *string
	if summary.EffectiveExpiry != nil {
		s := summary.EffectiveExpiry.UTC().Format(time.RFC3339)
		expStr = &s
	}

	return c.Status(fiber.StatusOK).JSON(SummaryResponse{
		IsActive:          summary.IsActive,
		CurrentMembership: toResponse(summary.CurrentMembership),
		EffectiveExpiry:   expStr,
		DaysRemaining:     summary.DaysRemaining,
		SecondsRemaining:  summary.SecondsRemaining,
		History:           historyResp,
	})
}

// PurchasePass handles POST /api/memberships/purchase.
// Confirms 30-day pass purchase by atomically debiting user personal wallet (FR-04, QR-04, QR-06).
func (h *Handler) PurchasePass(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized: authentication required",
		})
	}

	var req PurchasePassRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	rawPlan := req.PlanType
	if rawPlan == "" {
		rawPlan = req.Plan
	}
	plan := PlanType(rawPlan)
	if plan == "" {
		plan = PlanTypeStandard30Day
	}
	if plan != PlanTypeStandard30Day {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid plan type; currently supported: STANDARD_30_DAY",
		})
	}

	idemKey := strings.TrimSpace(req.IdempotencyKey)
	if idemKey == "" {
		idemKey = uuid.New().String()
	}

	ctx, cancel := context.WithTimeout(c.Context(), h.queryTimeout)
	defer cancel()

	mbr, err := h.service.PurchaseOrRenewPass(ctx, principal.User.ID, plan, idemKey)
	if err != nil {
		if errors.Is(err, ErrInsufficientFunds) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error": "insufficient wallet funds for gym membership purchase",
			})
		}
		if errors.Is(err, ErrInvalidPlanType) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		if errors.Is(err, ErrIdempotencyKeyRequired) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to process gym membership purchase",
		})
	}

	return c.Status(fiber.StatusOK).JSON(toResponse(mbr))
}

// ListMembershipsAdmin handles GET /api/admin/memberships.
func (h *Handler) ListMembershipsAdmin(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	ctx, cancel := context.WithTimeout(c.Context(), h.queryTimeout)
	defer cancel()

	list, err := h.service.ListAllMemberships(ctx, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to list memberships",
		})
	}

	var resp []*MembershipResponse
	for _, m := range list {
		resp = append(resp, toResponse(m))
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}
