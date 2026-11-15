package padel

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
	"giki-sports-backend/internal/wallet"
)

// Handler handles public and administrative HTTP endpoints for the Padel Court Booking inventory engine.
type Handler struct {
	service      *Service
	queryTimeout time.Duration
}

// NewHandler constructs a Handler with the specified service and query timeout.
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

// RegisterPublicRoutes mounts public, read-only padel endpoints on the supplied Fiber router.
func (h *Handler) RegisterPublicRoutes(router fiber.Router) {
	router.Get("/padel/facilities", h.ListFacilities)
	router.Get("/padel/slots", h.GetSlots)
}

// RegisterAuthenticatedRoutes mounts protected endpoints requiring an authenticated session.
func (h *Handler) RegisterAuthenticatedRoutes(
	router fiber.Router,
	requireOrigin fiber.Handler,
	requireAuth fiber.Handler,
) {
	router.Post("/padel/slots/:id/hold", requireOrigin, requireAuth, h.HoldSlot)
	router.Post("/padel/slots/:id/release-hold", requireOrigin, requireAuth, h.ReleaseHold)
	router.Post("/padel/checkout", requireOrigin, requireAuth, h.Checkout)
	router.Post("/padel/bookings/:id/cancel", requireOrigin, requireAuth, h.RejectUserCancellation)
}

// RegisterAdminRoutes mounts protected administrative slot generation, ticket verification, and booking cancellation endpoints.
func (h *Handler) RegisterAdminRoutes(
	router fiber.Router,
	requireOrigin fiber.Handler,
	requireAuth fiber.Handler,
	requireRole fiber.Handler,
) {
	router.Post("/admin/padel/slots/generate", requireOrigin, requireAuth, requireRole, h.GenerateSlots)
	router.Patch("/admin/padel/slots/:id", requireOrigin, requireAuth, requireRole, h.UpdateSlot)
	router.Post("/padel/bookings/verify-ticket", requireOrigin, requireAuth, requireRole, h.VerifyTicket)
	router.Post("/admin/bookings/:id/cancel", requireOrigin, requireAuth, requireRole, h.CancelBooking)
	router.Post("/admin/padel/bookings/:id/cancel", requireOrigin, requireAuth, requireRole, h.CancelBooking)
}

func requestContext(c *fiber.Ctx, timeout time.Duration) (context.Context, context.CancelFunc) {
	if c.Context() != nil {
		return context.WithTimeout(c.Context(), timeout)
	}
	return context.WithTimeout(context.Background(), timeout)
}

// ListFacilities returns all facilities currently eligible for Padel games.
func (h *Handler) ListFacilities(c *fiber.Ctx) error {
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	facilities, err := h.service.GetPadelFacilities(ctx)
	if err != nil {
		log.Printf("[ERROR] Failed to list padel facilities: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}
	return c.JSON(facilities)
}

// GetSlots handles GET /api/padel/slots?date=YYYY-MM-DD&facilityId=<uuid>.
//
// Parameters:
// - date: required calendar date in YYYY-MM-DD format.
// - facilityId: optional UUID. If omitted, automatically resolves to the primary Padel court facility.
//
// Authentication:
// - Public read endpoint.
// - If the request presents a valid session cookie, the viewer's user ID is used to populate `isHeldByMe`.
func (h *Handler) GetSlots(c *fiber.Ctx) error {
	dateStr := strings.TrimSpace(c.Query("date"))
	if dateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "missing required query parameter: date (expected YYYY-MM-DD)",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	var facUUID uuid.UUID
	facilityIDParam := strings.TrimSpace(c.Query("facilityId"))

	if facilityIDParam != "" {
		parsed, err := uuid.Parse(facilityIDParam)
		if err != nil || parsed == uuid.Nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "invalid facilityId query parameter: must be a valid UUID",
			})
		}
		facUUID = parsed
	} else {
		resolvedID, err := h.service.GetDefaultFacilityID(ctx)
		if err != nil {
			if errors.Is(err, ErrFacilityNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
					Error: "no padel court facility is configured in the system",
				})
			}
			log.Printf("[ERROR] Failed to resolve default padel facility: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
		facUUID = resolvedID
	}

	// Check for optional authenticated session principal to set isHeldByMe
	var viewerUserID *uuid.UUID
	if principal, ok := authentication.GetPrincipal(c); ok && principal != nil && principal.User != nil && principal.User.ID != uuid.Nil {
		viewerUserID = &principal.User.ID
	}

	resp, err := h.service.GetSlotsForDate(ctx, facUUID, dateStr, viewerUserID)
	if err != nil {
		if errors.Is(err, ErrInvalidDate) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "invalid date parameter: expected format YYYY-MM-DD",
			})
		}
		if errors.Is(err, ErrFacilityNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "specified facility not found or inactive",
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Failed to retrieve padel slots for facility %s on %s: %v", facUUID, dateStr, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.JSON(resp)
}

// GenerateSlots handles POST /api/admin/padel/slots/generate.
// Allows administrators to bulk-generate daily time inventory slots.
func (h *Handler) GenerateSlots(c *fiber.Ctx) error {
	var req GenerateSlotsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid JSON request payload",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.GenerateDailySlots(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDate),
			errors.Is(err, ErrInvalidOperatingHours),
			errors.Is(err, ErrInvalidSlotDuration),
			errors.Is(err, ErrNegativePrice):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		case errors.Is(err, ErrFacilityNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "facility not found or inactive",
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Failed to generate padel slots: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

// UpdateSlot handles PATCH /api/admin/padel/slots/:id.
// Allows sports administrators to adjust slot price or status.
func (h *Handler) UpdateSlot(c *fiber.Ctx) error {
	idParam := strings.TrimSpace(c.Params("id"))
	slotID, err := uuid.Parse(idParam)
	if err != nil || slotID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid slot ID parameter: must be a valid UUID",
		})
	}

	var req UpdateSlotRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid JSON request payload",
		})
	}

	if req.PricePaisa == nil && req.Status == nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "at least one field (pricePaisa or status) must be provided for update",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	updatedSlot, err := h.service.UpdateSlot(ctx, slotID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrSlotNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "slot not found",
			})
		case errors.Is(err, ErrSlotBooked):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "cannot modify a slot that is already booked",
			})
		case errors.Is(err, ErrNegativePrice), errors.Is(err, ErrInvalidStatus):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Failed to update padel slot %s: %v", slotID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	return c.JSON(updatedSlot)
}

// HoldSlot handles POST /api/padel/slots/:id/hold.
// Acquires an exclusive 5-minute hold on a slot for the authenticated principal.
// Security:
// - User ID is derived strictly from the authenticated session (Fiber locals).
// - Rejects concurrent holds on the same active slot with HTTP 409 Conflict.
func (h *Handler) HoldSlot(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	slotID, err := uuid.Parse(idParam)
	if err != nil || slotID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid slot ID parameter: must be a valid UUID",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.HoldSlot(ctx, slotID, principal.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrSlotConflict):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "slot is currently held by another user",
			})
		case errors.Is(err, ErrSlotUnavailable):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "slot is not available for booking",
			})
		case errors.Is(err, ErrSlotNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "slot not found",
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Failed to hold padel slot %s for user %s: %v", slotID, principal.User.ID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	return c.JSON(resp)
}

// ReleaseHold handles POST /api/padel/slots/:id/release-hold.
// Voluntarily cancels an active hold owned by the authenticated principal.
func (h *Handler) ReleaseHold(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	slotID, err := uuid.Parse(idParam)
	if err != nil || slotID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid slot ID parameter: must be a valid UUID",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.ReleaseHold(ctx, slotID, principal.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrHoldNotOwned):
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error: "cannot release a hold owned by another user",
			})
		case errors.Is(err, ErrSlotNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "slot not found",
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}
	}

	return c.JSON(resp)
}

// Checkout handles POST /api/padel/checkout.
// Confirms a padel court booking by atomically debiting the user's wallet balance
// and issuing a confirmed reservation ticket (FR-03, QR-01, QR-03, QR-04).
func (h *Handler) Checkout(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	var req CheckoutRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid request body",
		})
	}

	if strings.TrimSpace(req.SlotID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "slot ID is required",
		})
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "idempotency key cannot be blank",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.Checkout(ctx, req, principal.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrBlankIdempotencyKey):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "idempotency key cannot be blank",
			})
		case errors.Is(err, ErrSlotNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "slot not found",
			})
		case errors.Is(err, ErrNotSlotHolder):
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error: "you do not have an active hold on this slot",
			})
		case errors.Is(err, ErrHoldExpired):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "hold on slot has expired",
			})
		case errors.Is(err, ErrSlotUnavailable):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "slot is not available for booking",
			})
		case errors.Is(err, ErrIdempotencyConflict):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "idempotency key reused with mismatched parameters",
			})
		case errors.Is(err, wallet.ErrInsufficientFunds):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "insufficient wallet balance",
			})
		case errors.Is(err, wallet.ErrWalletNotFound):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "personal wallet not found",
			})
		case errors.Is(err, wallet.ErrWalletInactive):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "wallet is inactive",
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Checkout failed for slot %s, user %s: %v", req.SlotID, principal.User.ID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// VerifyTicket handles POST /api/padel/bookings/verify-ticket.
// Allows authorized complex attendants and directors to verify a 4-digit check-in code at court entrance (FR-03, TC-FR-03-03).
func (h *Handler) VerifyTicket(c *fiber.Ctx) error {
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	var req VerifyTicketRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid JSON request payload",
		})
	}

	if strings.TrimSpace(req.CheckInCode) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "check-in code cannot be blank",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.VerifyTicket(ctx, req, principal.User.ID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCheckInCode):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "invalid or empty check-in code",
			})
		case errors.Is(err, ErrBookingNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "booking not found with the provided check-in code",
			})
		case errors.Is(err, ErrTicketAlreadyCheckedIn):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "ticket has already been checked in",
			})
		case errors.Is(err, ErrBookingCancelled):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "cannot check in a cancelled booking",
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Ticket verification failed for code %s: %v", req.CheckInCode, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// RejectUserCancellation implements policy rule FR-06 rejecting self-service cancellation by standard users.
func (h *Handler) RejectUserCancellation(c *fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
		Error: "user self-cancellation is not permitted under sports complex policy; please contact an administrator",
	})
}

// CancelBooking handles administrative booking cancellation and full wallet refund (FR-06, TC-FR-06-02).
func (h *Handler) CancelBooking(c *fiber.Ctx) error {
	idParam := c.Params("id")
	bookingID, err := uuid.Parse(strings.TrimSpace(idParam))
	if err != nil || bookingID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid booking ID format",
		})
	}

	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "authentication required",
		})
	}

	var req CancelBookingRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "invalid request payload",
			})
		}
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	resp, err := h.service.CancelBooking(ctx, bookingID, principal.User.ID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrBookingNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: err.Error(),
			})
		case errors.Is(err, ErrBookingAlreadyCancelled):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: err.Error(),
			})
		case errors.Is(err, ErrCannotCancelCheckedInBooking):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: err.Error(),
			})
		case errors.Is(err, context.DeadlineExceeded):
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		default:
			log.Printf("[ERROR] Failed to cancel booking %s: %v", bookingID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}
