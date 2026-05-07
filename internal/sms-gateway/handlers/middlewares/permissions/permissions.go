package permissions

import (
	"slices"

	"github.com/gofiber/fiber/v2"
)

type contextKey string

const (
	ScopeAll   = "all:any"
	ScopeAdmin = "admin:all"

	localsScopes = contextKey("scopes")
)

func SetScopes(c *fiber.Ctx, scopes []string) {
	c.Locals(localsScopes, scopes)
}

func HasScope(c *fiber.Ctx, scope string, opts *options) bool {
	if opts == nil {
		opts = defaultOptions()
	}

	scopes, ok := c.Locals(localsScopes).([]string)
	if !ok {
		return false
	}

	return slices.ContainsFunc(
		scopes,
		func(item string) bool { return item == scope || (!opts.exact && item == ScopeAll) },
	)
}

// IsAdmin returns true if the user has the admin:all scope.
func IsAdmin(c *fiber.Ctx) bool {
	return HasScope(c, ScopeAdmin, nil)
}

// AdminUserID is a sentinel value that means "admin access, no user filter".
// It is never a valid real userID because real IDs are 6-char uppercase alphanumeric.
const AdminUserID = "__ADMIN__"

// EffectiveUserID returns the userID for data filtering.
// If the user has admin:all scope, returns AdminUserID sentinel.
// Otherwise returns the actual userID.
func EffectiveUserID(c *fiber.Ctx, userID string) string {
	if IsAdmin(c) {
		return AdminUserID
	}
	return userID
}

func RequireScope(scope string, opts ...Option) fiber.Handler {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	return func(c *fiber.Ctx) error {
		if !HasScope(c, scope, o) {
			return fiber.NewError(fiber.StatusForbidden, "scope required: "+scope)
		}

		return c.Next()
	}
}
