package authentication

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/session"
)

// CookieManager encapsulates centralized session cookie issuance and deletion rules.
type CookieManager struct {
	name     string
	path     string
	domain   string
	secure   bool
	sameSite string
	maxAge   int
}

// NewCookieManager constructs a CookieManager adhering to centralized security policies.
func NewCookieManager(cfg config.AuthConfig) *CookieManager {
	maxAge := int(session.AbsoluteLifetime.Seconds())
	if maxAge <= 0 {
		maxAge = 28800 // 8 hours fallback
	}

	return &CookieManager{
		name:     cfg.CookieName,
		path:     cfg.CookiePath,
		domain:   "", // Explicitly empty: no Domain attribute to prevent broad subdomain scoping
		secure:   cfg.CookieSecure,
		sameSite: cfg.CookieSameSite,
		maxAge:   maxAge,
	}
}

// BuildCookie constructs a hardened fiber.Cookie with MaxAge and Expires strictly bounded
// by absoluteExpiresAt, ensuring neither extends beyond that instant.
// Rejects already-expired sessions or timestamps that round to <= 0 integer seconds.
func (cm *CookieManager) BuildCookie(rawToken string, absoluteExpiresAt time.Time, now time.Time) (*fiber.Cookie, error) {
	remaining := absoluteExpiresAt.Sub(now)
	if remaining <= 0 {
		return nil, errors.New("cannot issue cookie for already expired session")
	}

	maxAgeSec := int(remaining.Seconds())
	if maxAgeSec <= 0 {
		return nil, errors.New("cannot issue cookie: session lifetime remaining rounds to zero seconds")
	}

	cookie := new(fiber.Cookie)
	cookie.Name = cm.name
	cookie.Value = rawToken
	cookie.Path = cm.path
	cookie.Domain = cm.domain
	cookie.MaxAge = maxAgeSec
	cookie.Expires = absoluteExpiresAt
	cookie.Secure = cm.secure
	cookie.HTTPOnly = true
	cookie.SameSite = cm.sameSite
	return cookie, nil
}

// SetSessionCookie sets a hardened HTTP-only session cookie containing the raw token.
// Strictly bounds cookie lifetime to the persisted session's absoluteExpiresAt instant.
func (cm *CookieManager) SetSessionCookie(c *fiber.Ctx, rawToken string, absoluteExpiresAt time.Time) error {
	cookie, err := cm.BuildCookie(rawToken, absoluteExpiresAt, time.Now())
	if err != nil {
		return err
	}
	c.Cookie(cookie)
	return nil
}

// ClearSessionCookie clears the session cookie using identical attributes and negative MaxAge.
func (cm *CookieManager) ClearSessionCookie(c *fiber.Ctx) {
	cookie := new(fiber.Cookie)
	cookie.Name = cm.name
	cookie.Value = ""
	cookie.Path = cm.path
	cookie.Domain = cm.domain
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(0, 0)
	cookie.Secure = cm.secure
	cookie.HTTPOnly = true
	cookie.SameSite = cm.sameSite
	c.Cookie(cookie)
}

// CookieName returns the configured cookie name.
func (cm *CookieManager) CookieName() string {
	return cm.name
}

// ValidateOrigin performs exact, canonical origin verification for mutation endpoints.
// Rejects missing, empty, "null", malformed, and unlisted origins.
// Never uses substring matching.
func ValidateOrigin(originHeader string, trustedOrigins []string) bool {
	if originHeader == "" || originHeader == "null" {
		return false
	}

	parsed, err := url.Parse(originHeader)
	if err != nil {
		return false
	}

	// Origin must be a valid absolute URI with http/https scheme and non-empty host
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}

	// Origin specification dictates that an origin has no path (or "/"), query, or fragment
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	canonicalOrigin := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)

	for _, trusted := range trustedOrigins {
		trustedParsed, err := url.Parse(strings.TrimSpace(trusted))
		if err != nil {
			continue
		}
		if (trustedParsed.Scheme != "http" && trustedParsed.Scheme != "https") || trustedParsed.Host == "" {
			continue
		}
		trustedCanonical := strings.ToLower(trustedParsed.Scheme) + "://" + strings.ToLower(trustedParsed.Host)
		if canonicalOrigin == trustedCanonical {
			return true
		}
	}

	return false
}
