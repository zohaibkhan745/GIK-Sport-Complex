package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"giki-sports-backend/internal/authentication"
)

// MaxDraftPayloadBytes limits the allowable request body size for draft authoring and editing (64KB).
const MaxDraftPayloadBytes = 64 * 1024

// Handler provides HTTP endpoints for public and administrative content operations.
type Handler struct {
	repo         Repository
	queryTimeout time.Duration
}

// NewHandler constructs a Handler with the specified repository and query timeout.
// If queryTimeout is non-positive, defaults to 5 seconds.
func NewHandler(repo Repository, queryTimeout time.Duration) *Handler {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	return &Handler{
		repo:         repo,
		queryTimeout: queryTimeout,
	}
}

// RegisterPublicRoutes mounts public, read-only content endpoints on the supplied Fiber router.
func (h *Handler) RegisterPublicRoutes(router fiber.Router) {
	router.Get("/content/announcements", h.ListAnnouncements)
	router.Get("/content/achievements", h.ListAchievements)
}

// RegisterDraftRoutes mounts protected draft management endpoints:
// - POST /admin/content/drafts (requires Origin validation, session auth, role check)
// - GET /admin/content/drafts/:id (requires session auth, role check; no Origin check)
// - PUT /admin/content/drafts/:id (requires Origin validation, session auth, role check)
// - POST /admin/content/drafts/:id/publish (requires Origin validation, session auth, role check)
func (h *Handler) RegisterDraftRoutes(
	router fiber.Router,
	requireOrigin fiber.Handler,
	requireAuth fiber.Handler,
	requireRole fiber.Handler,
) {
	router.Post("/admin/content/drafts", requireOrigin, requireAuth, requireRole, h.CreateDraft)
	router.Get("/admin/content/drafts/:id", requireAuth, requireRole, h.GetDraftByID)
	router.Put("/admin/content/drafts/:id", requireOrigin, requireAuth, requireRole, h.UpdateDraft)
	router.Post("/admin/content/drafts/:id/publish", requireOrigin, requireAuth, requireRole, h.PublishDraft)
}

// RegisterPublishRoute mounts the protected POST /admin/content/drafts/:id/publish endpoint.
func (h *Handler) RegisterPublishRoute(
	router fiber.Router,
	requireOrigin fiber.Handler,
	requireAuth fiber.Handler,
	requireRole fiber.Handler,
) {
	router.Post("/admin/content/drafts/:id/publish", requireOrigin, requireAuth, requireRole, h.PublishDraft)
}

// RegisterRoutes mounts public content endpoints on the supplied Fiber router.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	h.RegisterPublicRoutes(router)
}

// ListAnnouncements handles GET /api/content/announcements.
func (h *Handler) ListAnnouncements(c *fiber.Ctx) error {
	return h.listByKind(c, KindAnnouncement)
}

// ListAchievements handles GET /api/content/achievements.
func (h *Handler) ListAchievements(c *fiber.Ctx) error {
	return h.listByKind(c, KindAchievement)
}

func (h *Handler) listByKind(c *fiber.Ctx, kind Kind) error {
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	items, err := h.repo.ListPublishedByKind(ctx, kind)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Content list query for %s timed out after %s: %v", kind, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Content list query for %s failed: %v", kind, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToPublicContentResponses(items))
}

// CreateDraft handles POST /api/admin/content/drafts.
// Requires session authentication and sports_director or super_admin role with a trusted Origin.
// Returns HTTP 201 Created with the created DraftResponse on success.
func (h *Handler) CreateDraft(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// 1. Content-Type Check: Require application/json
	ct := c.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType != "application/json" {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(ErrorResponse{
			Error: "unsupported media type: Content-Type must be application/json",
		})
	}

	// 2. Body Size Bound: Reject empty or payloads exceeding 64KB
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "request body cannot be empty",
		})
	}
	if len(body) > MaxDraftPayloadBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error: "request body too large",
		})
	}

	// 3. Strict JSON Decoding: reject unknown fields (e.g. client actor, status, published_at)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req CreateDraftRequest
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

	// 4. Map to domain input and validate contract rules
	input := CreateDraftInput{
		Kind:    Kind(req.Kind),
		Title:   req.Title,
		Content: req.Content,
	}
	if err := input.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: err.Error(),
		})
	}

	// 5. Extract Actor ID from authenticated principal
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

	// 6. Execute draft creation under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	draft, err := h.repo.CreateDraft(ctx, input, audit)
	if err != nil {
		var valErr *DraftValidationError
		if errors.As(err, &valErr) || errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrInvalidKind) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Draft creation timed out after %s: %v", h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Draft creation failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(ToDraftResponse(*draft))
}

// GetDraftByID handles GET /api/admin/content/drafts/:id.
// Requires session authentication and sports_director or super_admin role.
// Returns HTTP 200 with DraftResponse, HTTP 404 if not found, or HTTP 409 if no longer a draft.
func (h *Handler) GetDraftByID(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil || id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid draft ID: must be a valid UUID",
		})
	}

	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	draft, err := h.repo.GetDraftByID(ctx, id)
	if err != nil {
		var notFoundErr *DraftNotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrDraftNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "draft not found",
			})
		}
		var notDraftErr *NotADraftError
		if errors.As(err, &notDraftErr) || errors.Is(err, ErrNotADraft) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "content item is not a draft",
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] GetDraftByID query for %s timed out after %s: %v", id, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] GetDraftByID failed for %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToDraftResponse(*draft))
}

// UpdateDraft handles PUT /api/admin/content/drafts/:id.
// Requires session authentication and sports_director or super_admin role with a trusted Origin.
// Replaces title and content (omitted or null fields clear that field). Preserves kind, creator, and creation time.
func (h *Handler) UpdateDraft(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil || id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid draft ID: must be a valid UUID",
		})
	}

	// 1. Content-Type Check: Require application/json
	ct := c.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	if mediaType != "application/json" {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(ErrorResponse{
			Error: "unsupported media type: Content-Type must be application/json",
		})
	}

	// 2. Body Size Bound: Reject empty or payloads exceeding 64KB
	body := c.Body()
	if len(body) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "request body cannot be empty",
		})
	}
	if len(body) > MaxDraftPayloadBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error: "request body too large",
		})
	}

	// 3. Strict JSON Decoding: reject unknown fields (e.g. kind, status, actor, timestamps)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req UpdateDraftRequest
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

	// 4. Map to domain input and validate contract rules
	input := UpdateDraftInput{
		Title:   req.Title,
		Content: req.Content,
	}
	if err := input.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: err.Error(),
		})
	}

	// 5. Extract Actor ID from authenticated principal
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

	// 6. Execute draft update under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	draft, err := h.repo.UpdateDraft(ctx, id, input, audit)
	if err != nil {
		var notFoundErr *DraftNotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrDraftNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "draft not found",
			})
		}
		var notDraftErr *NotADraftError
		if errors.As(err, &notDraftErr) || errors.Is(err, ErrNotADraft) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "content item is not a draft",
			})
		}
		var valErr *DraftValidationError
		if errors.As(err, &valErr) || errors.Is(err, ErrInvalidInput) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Draft update for %s timed out after %s: %v", id, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		log.Printf("[ERROR] Draft update failed for %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToDraftResponse(*draft))
}

// PublishDraft handles POST /api/admin/content/drafts/:id/publish.
// Requires session authentication and sports_director or super_admin role with a trusted Origin.
// The request accepts no payload; publishes the draft identified by :id.
// Returns HTTP 200 with PublicContentResponse on successful publication and audit commit.
func (h *Handler) PublishDraft(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// 1. Validate UUID parameter
	idParam := c.Params("id")
	id, err := uuid.Parse(idParam)
	if err != nil || id == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "invalid draft ID: must be a valid UUID",
		})
	}

	// 2. Reject unexpected request body (publish accepts no payload)
	if len(bytes.TrimSpace(c.Body())) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "unexpected request body: publish endpoint accepts no payload",
		})
	}

	// 3. Extract Actor ID from authenticated principal
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

	// 4. Execute audited publication under bounded context deadline
	ctx, cancel := requestContext(c, h.queryTimeout)
	defer cancel()

	item, err := h.repo.PublishDraft(ctx, id, audit)
	if err != nil {
		// Map incomplete draft to 422 Unprocessable Entity
		// Note: IncompleteDraftError also satisfies ErrInvalidInput, so check ErrIncompleteDraft first.
		var incompleteErr *IncompleteDraftError
		if errors.As(err, &incompleteErr) || errors.Is(err, ErrIncompleteDraft) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}

		var notFoundErr *DraftNotFoundError
		if errors.As(err, &notFoundErr) || errors.Is(err, ErrDraftNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error: "draft not found",
			})
		}

		var notDraftErr *NotADraftError
		if errors.As(err, &notDraftErr) || errors.Is(err, ErrNotADraft) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error: "content item is not a draft",
			})
		}

		var valErr *DraftValidationError
		if errors.As(err, &valErr) || errors.Is(err, ErrInvalidInput) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: err.Error(),
			})
		}

		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[WARN] Draft publish query for %s timed out after %s: %v", id, h.queryTimeout, err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}

		log.Printf("[ERROR] Draft publish failed for %s: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToPublicContentResponse(*item))
}

func requestContext(c *fiber.Ctx, timeout time.Duration) (context.Context, context.CancelFunc) {
	parent := c.UserContext()
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}
