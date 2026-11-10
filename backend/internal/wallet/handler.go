package wallet

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
)

// Handler provides HTTP endpoints for wallet queries and ledger views.
type Handler struct {
	service      *Service
	queryTimeout time.Duration
}

// NewHandler constructs a Handler with the given service and bounded query timeout.
// If queryTimeout is non-positive, defaults to 5 seconds.
func NewHandler(service *Service, queryTimeout time.Duration) *Handler {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	return &Handler{
		service:      service,
		queryTimeout: queryTimeout,
	}
}

// RegisterRoutes registers wallet endpoints onto the provided Fiber router.
// Route: GET /wallet/me requires a valid session (RequireAuth).
func (h *Handler) RegisterRoutes(router fiber.Router, requireAuth fiber.Handler) {
	router.Get("/wallet/me", requireAuth, h.GetMyWallet)
}

// GetMyWallet handles GET /api/wallet/me.
//
// Security & Invariants:
// - Ownership check: User ID is extracted strictly from the authenticated principal in Fiber locals.
//   Client-supplied query parameters, path variables, or headers (e.g. X-User-ID) are ignored.
// - No administrative role is required; any authenticated user can read their own personal wallet.
// - Cache-Control: no-store and Pragma: no-cache are set to prevent intermediate and browser caching.
// - Bounded execution context protects against hanging queries.
// - Strictly read-only: does not create a wallet or insert ledger records.
// - Distinguishes "no wallet exists" (returns 200 with wallet: null, activity: []) from an existing wallet
//   with zero balance (returns 200 with wallet: {status, currency, balancePaisa: "0"}, activity: []).
// - Monetary amounts are serialized as decimal strings in integer paisa (e.g. "40000", "-5000") to prevent
//   precision loss in JavaScript (which limits exact integers to 2^53 - 1).
// - Privacy: Counterparty information, system balances, and internal reference IDs are not exposed.
//
// CONCURRENCY SNAPSHOT CAVEAT:
// Balance and recent activity are derived via independent database queries under Read Committed isolation.
// If a financial transfer commits concurrently between the balance calculation and the activity fetch,
// the balance and the activity list may reflect slightly different committed states. Callers requiring
// an atomic point-in-time snapshot must execute both queries within a coordinated read-only transaction.
func (h *Handler) GetMyWallet(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	userID := principal.User.ID

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	// 1. Query personal wallet balance
	balanceInfo, err := h.service.GetUserWalletBalance(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrWalletNotFound) {
			// No wallet exists yet: return 200 with wallet: null and activity: []
			return c.Status(fiber.StatusOK).JSON(WalletMeResponse{
				Wallet:   nil,
				Activity: make([]WalletActivityDTO, 0),
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Wallet balance query for user %s timed out after %s: %v", userID, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Wallet balance query for user %s failed: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	// 2. Query personal wallet recent activity (latest 50 entries)
	historyItems, err := h.service.GetUserWalletHistory(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrWalletNotFound) {
			// Edge case: wallet was concurrently removed
			return c.Status(fiber.StatusOK).JSON(WalletMeResponse{
				Wallet:   nil,
				Activity: make([]WalletActivityDTO, 0),
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Wallet history query for user %s timed out after %s: %v", userID, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Wallet history query for user %s failed: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	// 3. Map to DTOs with exact decimal string serialization for paisa amounts
	walletDTO := &WalletSummaryDTO{
		Status:       string(balanceInfo.Status),
		Currency:     balanceInfo.Currency,
		BalancePaisa: strconv.FormatInt(balanceInfo.Balance, 10),
	}

	activityDTOs := make([]WalletActivityDTO, 0, len(historyItems))
	for _, item := range historyItems {
		activityDTOs = append(activityDTOs, WalletActivityDTO{
			EntryID:       item.EntryID.String(),
			TransactionID: item.TransactionID.String(),
			Type:          string(item.Type),
			AmountPaisa:   strconv.FormatInt(item.Amount, 10),
			CreatedAt:     item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return c.Status(fiber.StatusOK).JSON(WalletMeResponse{
		Wallet:   walletDTO,
		Activity: activityDTOs,
	})
}

func requestContext(c *fiber.Ctx, timeout time.Duration) (context.Context, context.CancelFunc) {
	parent := c.UserContext()
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

