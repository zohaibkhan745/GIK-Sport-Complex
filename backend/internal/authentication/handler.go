package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/user"
)

// Authenticator defines the domain service operations required by the authentication HTTP handlers.
// Consumer-owned interface enabling small test fakes without concrete repository dependencies.
type Authenticator interface {
	Login(ctx context.Context, email, password string) (*LoginResult, error)
	ValidateSession(ctx context.Context, rawToken string) (*AuthenticatedPrincipal, error)
	Logout(ctx context.Context, rawToken string) error
}

// Handler orchestrates HTTP endpoints for user authentication and session lifecycle.
type Handler struct {
	service        Authenticator
	cookieManager  *CookieManager
	trustedOrigins []string
	requestTimeout time.Duration
	concurrencySem chan struct{}
	rateLimiter    fiber.Handler
}

// NewHandler constructs a Handler backed by the provided Authenticator and AuthConfig.
func NewHandler(service Authenticator, cfg config.AuthConfig) *Handler {
	cookieMgr := NewCookieManager(cfg)

	maxConcurrent := cfg.LoginMaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}

	reqTimeout := cfg.RequestTimeout
	if reqTimeout <= 0 {
		reqTimeout = 5 * time.Second
	}

	rateLimit := cfg.LoginRateLimit
	if rateLimit <= 0 {
		rateLimit = 10
	}

	rateWindow := cfg.LoginRateWindow
	if rateWindow <= 0 {
		rateWindow = 1 * time.Minute
	}

	// In-memory per-client IP rate limiter for login attempts
	rateLimiterMiddleware := limiter.New(limiter.Config{
		Max:        rateLimit,
		Expiration: rateWindow,
		KeyGenerator: func(c *fiber.Ctx) string {
			// Fiber c.IP() returns the direct remote IP unless trusted proxies are explicitly configured
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			return c.Status(fiber.StatusTooManyRequests).JSON(ErrorResponse{
				Error: "too many login attempts; please try again later",
			})
		},
	})

	return &Handler{
		service:        service,
		cookieManager:  cookieMgr,
		trustedOrigins: cfg.TrustedOrigins,
		requestTimeout: reqTimeout,
		concurrencySem: make(chan struct{}, maxConcurrent),
		rateLimiter:    rateLimiterMiddleware,
	}
}

// RegisterRoutes registers the authentication HTTP endpoints on the provided Fiber router.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	authGroup := router.Group("/auth")
	authGroup.Post("/login", h.RequireTrustedOrigin(), h.rateLimiter, h.Login)
	authGroup.Get("/me", h.RequireAuth(), h.Me)
	authGroup.Post("/logout", h.RequireTrustedOrigin(), h.Logout)
}

// Login handles POST /api/auth/login.
// Authenticates credentials, sets the hardened session cookie, and returns the public user profile.
func (h *Handler) Login(c *fiber.Ctx) error {
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

	// 2. Body Size Bound: Reject payloads larger than 4KB
	body := c.Body()
	if len(body) > 4096 {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error: "request body too large",
		})
	}

	// 3. JSON Payload Parsing
	var req LoginRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "malformed request payload",
		})
	}

	// 4. Technical Input Limits (do not trim password; preserve exact byte sequence)
	if len(req.Email) > 255 || len(req.Password) > 4096 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error: "input exceeds allowable length",
		})
	}

	// 5. Concurrency Control: Bounded concurrency semaphore for CPU/RAM-heavy Argon2id hashing
	select {
	case h.concurrencySem <- struct{}{}:
		defer func() { <-h.concurrencySem }()
	default:
		c.Set("Retry-After", "1")
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error: "authentication service is busy; please retry shortly",
		})
	}

	// 6. Bounded Context & Service Invocation
	ctx, cancel := context.WithTimeout(c.Context(), h.requestTimeout)
	defer cancel()

	res, err := h.service.Login(ctx, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Error: "invalid email or password",
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
				Error: "service unavailable: request timed out",
			})
		}
		// Redacted diagnostic: never print passwords or raw credentials
		log.Printf("[ERROR] Authentication failure for %q: %v", user.NormalizeEmail(req.Email), err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	// 7. Set Session Cookie strictly bounded by session's AbsoluteExpiresAt
	if err := h.cookieManager.SetSessionCookie(c, res.RawToken, res.Session.AbsoluteExpiresAt); err != nil {
		log.Printf("[ERROR] Failed to set session cookie for %q: %v", user.NormalizeEmail(req.Email), err)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "internal server error",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToUserResponse(res.User))
}

// Me handles GET /api/auth/me.
// Protected by RequireAuth; reads the validated principal established in Fiber locals.
func (h *Handler) Me(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	principal, ok := GetPrincipal(c)
	if !ok || principal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error: "unauthorized: missing session cookie",
		})
	}

	return c.Status(fiber.StatusOK).JSON(ToUserResponse(principal.User))
}

// Logout handles POST /api/auth/logout.
// Independent of RequireAuth so expired/revoked sessions can still be revoked and cookies cleared.
func (h *Handler) Logout(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("Pragma", "no-cache")

	// Extract Token and Revoke Server-Side Session
	rawToken := c.Cookies(h.cookieManager.CookieName())
	if rawToken != "" {
		ctx, cancel := context.WithTimeout(c.Context(), h.requestTimeout)
		defer cancel()

		if err := h.service.Logout(ctx, rawToken); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
					Error: "service unavailable: request timed out",
				})
			}
			log.Printf("[ERROR] Session logout revocation failure: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}
	}

	// Clear Session Cookie from Client
	h.cookieManager.ClearSessionCookie(c)
	return c.SendStatus(fiber.StatusNoContent)
}
