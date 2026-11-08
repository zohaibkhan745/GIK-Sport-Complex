package facility

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
)

// Reader defines the read-only persistence operations required by facility HTTP handlers.
type Reader interface {
	List(ctx context.Context) ([]Facility, error)
	GetBySlug(ctx context.Context, slug string) (*Facility, error)
}

// UpdateScheduleRequest defines the expected JSON payload for updating a recurring schedule window.
type UpdateScheduleRequest struct {
	DayOfWeek int    `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Audience  string `json:"audience"`
}

// CreateScheduleRequest defines the expected JSON payload for creating a recurring schedule window.
type CreateScheduleRequest struct {
	DayOfWeek int    `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Audience  string `json:"audience"`
}

// Handler provides HTTP handlers for facility read and write endpoints.
type Handler struct {
	repo         Repository
	queryTimeout time.Duration
}

// NewHandler constructs a Handler with the specified repository and query timeout.
func NewHandler(repo Repository, queryTimeout time.Duration) *Handler {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	return &Handler{
		repo:         repo,
		queryTimeout: queryTimeout,
	}
}

// RegisterRoutes mounts public facility read endpoints on the specified Fiber router.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Get("/facilities", h.List)
	router.Get("/facilities/:slug", h.GetBySlug)
}

// RegisterScheduleUpdateRoute mounts the protected PUT /facilities/:slug/schedules/:id endpoint
// preceded by the supplied middleware chain.
func (h *Handler) RegisterScheduleUpdateRoute(router fiber.Router, middleware ...fiber.Handler) {
	handlers := make([]fiber.Handler, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, h.UpdateSchedule)
	router.Put("/facilities/:slug/schedules/:id", handlers...)
}

// RegisterScheduleDeleteRoute mounts the protected DELETE /facilities/:slug/schedules/:id endpoint
// preceded by the supplied middleware chain.
func (h *Handler) RegisterScheduleDeleteRoute(router fiber.Router, middleware ...fiber.Handler) {
	handlers := make([]fiber.Handler, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, h.DeleteSchedule)
	router.Delete("/facilities/:slug/schedules/:id", handlers...)
}

// RegisterScheduleCreateRoute mounts the protected POST /facilities/:slug/schedules endpoint
// preceded by the supplied middleware chain.
func (h *Handler) RegisterScheduleCreateRoute(router fiber.Router, middleware ...fiber.Handler) {
	handlers := make([]fiber.Handler, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, h.CreateSchedule)
	router.Post("/facilities/:slug/schedules", handlers...)
}

// List handles GET /api/facilities.
func (h *Handler) List(c *fiber.Ctx) error {
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	facilities, err := h.repo.List(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Facility list query timed out after %s: %v", h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Facility list failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToFacilityResponses(facilities))
}

// GetBySlug handles GET /api/facilities/:slug.
func (h *Handler) GetBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		slug = c.Params("id")
	}
	if slug == "" {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error: "facility not found",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	fac, err := h.repo.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "facility not found",
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Facility detail query for %q timed out after %s: %v", slug, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Facility detail query for %q failed: %v", slug, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToFacilityResponse(*fac))
}

// UpdateSchedule handles PUT /api/facilities/:slug/schedules/:id.
// It requires an authenticated principal holding sports_director or super_admin role,
// validates the request payload, and executes an audited timetable update in a single transaction.
func (h *Handler) UpdateSchedule(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// 1. Validate facility slug
	rawSlug := c.Params("slug")
	slug, err := url.PathUnescape(rawSlug)
	if err != nil {
		slug = rawSlug
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error: "facility not found",
		})
	}

	// 2. Validate schedule UUID
	scheduleIDParam := c.Params("id")
	scheduleID, err := uuid.Parse(scheduleIDParam)
	if err != nil || scheduleID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid schedule id: must be a valid non-nil UUID",
		})
	}

	// 3. Content-Type Check: Require application/json
	ct := c.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType != "application/json" {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(ErrorResponse{
			Error: "unsupported media type: Content-Type must be application/json",
		})
	}

	// 4. Body Size Bound: Reject empty or payloads larger than 4KB
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "request body cannot be empty",
		})
	}
	if len(body) > 4096 {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error: "request body too large",
		})
	}

	// 5. JSON Parsing with DisallowUnknownFields to reject unexpected fields
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req UpdateScheduleRequest
	if err := dec.Decode(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: fmt.Sprintf("malformed request payload: %v", err),
		})
	}
	if dec.More() {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "malformed request payload: unexpected extra tokens after JSON object",
		})
	}

	// 6. Map to domain input and validate contract rules
	input := UpdateScheduleInput{
		DayOfWeek: req.DayOfWeek,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Audience:  req.Audience,
	}
	if err := input.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: err.Error(),
		})
	}

	// 7. Extract Actor ID from authenticated principal (never from headers or body)
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	// Optional IP and user-agent left unset for this chunk per specification
	audit := AuditParams{
		ActorID:   principal.User.ID,
		IPAddress: "",
		UserAgent: "",
	}

	// 8. Execute audited update under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	saved, err := h.repo.UpdateScheduleWithAudit(ctx, slug, scheduleID, input, audit)
	if err != nil {
		var valErr *ScheduleValidationError
		if errors.As(err, &valErr) || errors.Is(err, ErrInvalidInput) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}

		var notFoundErr *ScheduleNotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrScheduleNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "schedule not found for facility",
			})
		}

		var dupErr *DuplicateScheduleError
		if errors.As(err, &dupErr) || errors.Is(err, ErrDuplicateSchedule) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "schedule window duplicates an existing window for this facility",
			})
		}

		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Schedule update for facility %q schedule %s timed out after %s: %v", slug, scheduleID, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}

		log.Printf("[ERROR] Schedule update failed for facility %q schedule %s: %v", slug, scheduleID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToScheduleResponse(*saved))
}

// DeleteSchedule handles DELETE /api/facilities/:slug/schedules/:id.
// It requires an authenticated principal holding sports_director or super_admin role,
// validates the facility slug and schedule UUID parameters, and executes an audited deletion.
// Returns HTTP 204 No Content with an empty body on successful commit.
func (h *Handler) DeleteSchedule(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// 1. Validate facility slug
	slug := strings.TrimSpace(c.Params("slug"))
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "facility slug cannot be blank",
		})
	}

	// 2. Validate schedule UUID
	scheduleIDParam := c.Params("id")
	scheduleID, err := uuid.Parse(scheduleIDParam)
	if err != nil || scheduleID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid schedule id: must be a valid non-nil UUID",
		})
	}

	// 3. Extract Actor ID from authenticated principal (never trust client headers or body)
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	audit := AuditParams{
		ActorID:   principal.User.ID,
		IPAddress: "",
		UserAgent: "",
	}

	// 4. Execute audited deletion under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	err = h.repo.DeleteScheduleWithAudit(ctx, slug, scheduleID, audit)
	if err != nil {
		var notFoundErr *ScheduleNotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrScheduleNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "schedule not found for facility",
			})
		}

		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Schedule deletion for facility %q schedule %s timed out after %s: %v", slug, scheduleID, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}

		log.Printf("[ERROR] Schedule deletion failed for facility %q schedule %s: %v", slug, scheduleID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// CreateSchedule handles POST /api/facilities/:slug/schedules.
// It requires an authenticated principal holding sports_director or super_admin role,
// validates the facility slug and request body, and executes an audited window creation.
// Returns HTTP 201 Created with the created ScheduleResponse shape on successful commit.
func (h *Handler) CreateSchedule(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// 1. Validate facility slug
	rawSlug := c.Params("slug")
	slug, err := url.PathUnescape(rawSlug)
	if err != nil {
		slug = rawSlug
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error: "facility not found",
		})
	}

	// 2. Content-Type Check: Require application/json
	ct := c.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType != "application/json" {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(ErrorResponse{
			Error: "unsupported media type: Content-Type must be application/json",
		})
	}

	// 3. Body Size Bound: Reject empty or payloads larger than 4KB
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "request body cannot be empty",
		})
	}
	if len(body) > 4096 {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error: "request body too large",
		})
	}

	// 4. JSON Parsing with DisallowUnknownFields to reject unexpected fields
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req CreateScheduleRequest
	if err := dec.Decode(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: fmt.Sprintf("malformed request payload: %v", err),
		})
	}
	if dec.More() {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "malformed request payload: unexpected extra tokens after JSON object",
		})
	}

	// 5. Map to domain input and validate contract rules
	input := CreateScheduleInput{
		DayOfWeek: req.DayOfWeek,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Audience:  req.Audience,
	}

	if err := input.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: err.Error(),
		})
	}

	// 6. Extract Actor ID from authenticated principal (never trust client headers or body)
	principal, ok := authentication.GetPrincipal(c)
	if !ok || principal == nil || principal.User == nil || principal.User.ID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: authentication required",
		})
	}

	audit := AuditParams{
		ActorID:   principal.User.ID,
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	}

	// 7. Execute audited creation under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	saved, err := h.repo.CreateScheduleWithAudit(ctx, slug, input, audit)
	if err != nil {
		var notFoundErr *NotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "facility not found",
			})
		}

		var valErr *ScheduleValidationError
		if errors.As(err, &valErr) || errors.Is(err, ErrInvalidInput) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}

		var dupErr *DuplicateScheduleError
		if errors.As(err, &dupErr) || errors.Is(err, ErrDuplicateSchedule) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "schedule window duplicates an existing window for this facility",
			})
		}

		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Schedule creation for facility %q timed out after %s: %v", slug, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}

		log.Printf("[ERROR] Schedule creation failed for facility %q: %v", slug, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(ToScheduleResponse(*saved))
}

func requestContext(c *fiber.Ctx, timeout time.Duration) (context.Context, context.CancelFunc) {
	parent := c.UserContext()
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}
