package authentication

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"giki-sports-backend/internal/user"
)

// contextKey defines a private unexported type for Fiber context locals keys,
// preventing accidental collisions with strings or external packages.
type contextKey struct{}

var principalLocalKey = contextKey{}

// GetPrincipal retrieves the authenticated principal from Fiber locals.
// Returns nil, false if the request has not been authenticated by RequireAuth.
func GetPrincipal(c *fiber.Ctx) (*AuthenticatedPrincipal, bool) {
	val := c.Locals(principalLocalKey)
	if val == nil {
		return nil, false
	}
	principal, ok := val.(*AuthenticatedPrincipal)
	return principal, ok && principal != nil
}

// SetPrincipalForTest attaches an AuthenticatedPrincipal to Fiber locals for testing purposes.
func SetPrincipalForTest(c *fiber.Ctx, principal *AuthenticatedPrincipal) {
	c.Locals(principalLocalKey, principal)
}

// RequireAuth returns a Fiber middleware that validates the session cookie and attaches
// the authenticated principal to Fiber locals.
//
// Lifecycle & Security Contracts:
// - Extracts session token from cookie using configured name.
// - Validates session via Authenticator.ValidateSession under PostgreSQL row lock.
// - Bounded context deadline prevents lingering database locks.
// - Missing, invalid, expired, revoked, or inactive sessions return generic 401 Unauthorized.
// - Unexpected infrastructure errors return generic 500; deadline expirations return 503.
// - Sets Cache-Control: no-store and Pragma: no-cache on all protected responses.
// - Stores principal in Fiber locals using a private unexported key.
// - Strictly ignores client-supplied headers (e.g. X-User-ID, X-Role) as proof of identity.
func RequireAuth(service Authenticator, cookieMgr *CookieManager, timeout time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		c.Set("Pragma", "no-cache")

		rawToken := c.Cookies(cookieMgr.CookieName())
		if rawToken == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Error: "unauthorized: missing session cookie",
			})
		}

		ctx, cancel := context.WithTimeout(c.Context(), timeout)
		defer cancel()

		principal, err := service.ValidateSession(ctx, rawToken)
		if err != nil {
			if errors.Is(err, ErrInvalidSession) {
				return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
					Error: "unauthorized: invalid or expired session",
				})
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
					Error: "service unavailable: request timed out",
				})
			}
			log.Printf("[ERROR] Session authentication failure: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error: "internal server error",
			})
		}

		c.Locals(principalLocalKey, principal)
		return c.Next()
	}
}

// RequireAuth returns the RequireAuth middleware configured with the Handler's service and cookie manager.
func (h *Handler) RequireAuth() fiber.Handler {
	return RequireAuth(h.service, h.cookieManager, h.requestTimeout)
}

// RequireAnyRole returns a Fiber middleware that restricts route access to principals holding
// at least one of the specified roles.
//
// Security Rules:
// - Requires a principal previously established by RequireAuth. If missing, returns 401 Unauthorized.
// - If the user has at least one matching role, execution proceeds to the next handler.
// - If the user has none of the required roles, returns 403 Forbidden.
// - Empty role configuration denies access (fails closed with 403 Forbidden).
// - Unknown or unlisted roles fail closed with 403 Forbidden.
// - SuperAdmin is NOT automatically granted access; each route must explicitly specify permitted roles.
// - Cashier authority is NOT granted through any of these roles.
// - Client headers or request bodies are strictly ignored as proof of role.
func RequireAnyRole(roles ...user.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, ok := GetPrincipal(c)
		if !ok || principal == nil {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Error: "unauthorized: authentication required",
			})
		}

		// Empty configuration: fail closed
		if len(roles) == 0 {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error: "forbidden: insufficient permissions",
			})
		}

		hasRole := false
		for _, required := range roles {
			if !isValidRole(required) {
				// Unknown role configuration: fail closed
				c.Set("Cache-Control", "no-store")
				c.Set("Pragma", "no-cache")
				return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
					Error: "forbidden: insufficient permissions",
				})
			}
			for _, userRole := range principal.User.Roles {
				if userRole == required {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error: "forbidden: insufficient permissions",
			})
		}

		return c.Next()
	}
}

func isValidRole(r user.Role) bool {
	switch r {
	case user.RoleStudent, user.RoleFaculty, user.RoleSportsDirector, user.RoleSuperAdmin:
		return true
	default:
		return false
	}
}

// RequireTrustedOrigin returns a Fiber middleware enforcing exact canonical Origin header verification
// for browser-initiated mutation endpoints.
func RequireTrustedOrigin(trustedOrigins []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := c.Get("Origin")
		if !ValidateOrigin(origin, trustedOrigins) {
			c.Set("Cache-Control", "no-store")
			c.Set("Pragma", "no-cache")
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error: "forbidden: untrusted or missing origin",
			})
		}
		return c.Next()
	}
}

// RequireTrustedOrigin returns the RequireTrustedOrigin middleware configured with the Handler's trusted origins.
func (h *Handler) RequireTrustedOrigin() fiber.Handler {
	return RequireTrustedOrigin(h.trustedOrigins)
}
