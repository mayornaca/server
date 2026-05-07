package thirdparty

import (
	"errors"
	"fmt"
	"time"

	"github.com/android-sms-gateway/client-go/smsgateway"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/base"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/jwtauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/permissions"
	"github.com/android-sms-gateway/server/internal/sms-gateway/handlers/middlewares/userauth"
	"github.com/android-sms-gateway/server/internal/sms-gateway/jwt"
	"github.com/android-sms-gateway/server/internal/sms-gateway/users"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type AuthHandler struct {
	base.Handler

	jwtSvc   jwt.Service
	usersSvc *users.Service
}

func NewAuthHandler(
	jwtSvc jwt.Service,
	usersSvc *users.Service,

	logger *zap.Logger,
	validator *validator.Validate,
) *AuthHandler {
	return &AuthHandler{
		Handler:  base.Handler{Logger: logger, Validator: validator},
		jwtSvc:   jwtSvc,
		usersSvc: usersSvc,
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword"     validate:"required,min=8"`
}

func (h *AuthHandler) patchPassword(userID string, c *fiber.Ctx) error {
	req := new(changePasswordRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.usersSvc.ChangePassword(c.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "contraseña actual incorrecta")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) Register(router fiber.Router) {
	router.Use(h.errorHandler)
	router.Post("/token", permissions.RequireScope(ScopeTokensManage), userauth.WithUserID(h.postToken))
	router.Post(
		"/token/refresh",
		permissions.RequireScope(ScopeTokensRefresh, permissions.WithExact()),
		h.postRefreshToken,
	)
	router.Delete("/token/:jti", permissions.RequireScope(ScopeTokensManage), userauth.WithUserID(h.deleteToken))
	// cloud-gesvial.19.1: require ScopeTokensManage for password change so a
	// narrow-scope token (e.g. tests:read) leaked from a third-party
	// integration can't be used to take over the account. The currentPassword
	// challenge is still in place — this only adds the scope guard.
	router.Patch("/password", permissions.RequireScope(ScopeTokensManage), userauth.WithUserID(h.patchPassword))
}

//	@Summary		Generate token
//	@Description	Generate new access token with specified scopes and ttl
//	@Security		ApiAuth
//	@Security		JWTAuth
//	@Tags			User, Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		smsgateway.TokenRequest		true	"Request"
//	@Success		201		{object}	smsgateway.TokenResponse	"Token"
//	@Failure		400		{object}	smsgateway.ErrorResponse	"Invalid request"
//	@Failure		401		{object}	smsgateway.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	smsgateway.ErrorResponse	"Forbidden"
//	@Failure		500		{object}	smsgateway.ErrorResponse	"Internal server error"
//	@Failure		501		{object}	smsgateway.ErrorResponse	"Not implemented"
//	@Router			/3rdparty/v1/auth/token [post]
//
// Generate token.
func (h *AuthHandler) postToken(userID string, c *fiber.Ctx) error {
	req := new(smsgateway.TokenRequest)
	if err := h.BodyParserValidator(c, req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// cloud-gesvial.22.0: aplicar scopes permitidos del usuario.
	// Si el usuario tiene `users.scopes` poblado en BD, intersectamos los
	// scopes solicitados con esa lista. Si NULL/vacío en BD, conservamos el
	// comportamiento legacy (todos los scopes solicitados se conceden).
	// Esto cierra la puerta a que un user con creds limitadas pida
	// admin:all y reciba JWT con todos los permisos.
	scopes := req.Scopes
	user, err := h.usersSvc.GetByUsername(userID)
	if err == nil && user != nil && user.AllowedScopes != nil {
		scopes = intersectScopes(req.Scopes, user.AllowedScopes)
		if len(scopes) == 0 {
			h.Logger.Warn("requested scopes denied by user policy",
				zap.String("userID", userID),
				zap.Strings("requested", req.Scopes),
				zap.Strings("allowed", user.AllowedScopes),
			)
			return fiber.NewError(fiber.StatusForbidden, "no scopes available for this user")
		}
	}

	pair, err := h.jwtSvc.GenerateTokenPair(
		c.Context(),
		userID,
		scopes,
		time.Duration(req.TTL)*time.Second, //nolint:gosec // validated in the service
	)
	if err != nil {
		return fmt.Errorf("failed to generate token pair: %w", err)
	}

	return c.Status(fiber.StatusCreated).JSON(smsgateway.TokenResponse{
		ID:           pair.Access.ID,
		TokenType:    "Bearer",
		AccessToken:  pair.Access.Token,
		RefreshToken: pair.Refresh.Token,
		ExpiresAt:    pair.Access.ExpiresAt,
	})
}

// intersectScopes devuelve los scopes que aparecen en ambas listas. La
// pertenencia es exacta (sin wildcard). cloud-gesvial.22.0.
func intersectScopes(requested, allowed []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		allowedSet[s] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, s := range requested {
		if _, ok := allowedSet[s]; ok {
			out = append(out, s)
		}
	}
	return out
}

//	@Summary		Refresh token
//	@Description	Refresh access token with specified refresh token
//	@Security		JWTAuth
//	@Tags			User, Auth
//	@Produce		json
//	@Success		201	{object}	smsgateway.TokenResponse	"Token"
//	@Failure		401	{object}	smsgateway.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	smsgateway.ErrorResponse	"Forbidden"
//	@Failure		500	{object}	smsgateway.ErrorResponse	"Internal server error"
//	@Failure		501	{object}	smsgateway.ErrorResponse	"Not implemented"
//	@Router			/3rdparty/v1/auth/token/refresh [post]
//
// Refresh token.
func (h *AuthHandler) postRefreshToken(c *fiber.Ctx) error {
	token := jwtauth.GetToken(c)

	pair, err := h.jwtSvc.RefreshTokenPair(c.Context(), token)
	if err != nil {
		return fmt.Errorf("failed to refresh token pair: %w", err)
	}

	return c.Status(fiber.StatusCreated).JSON(smsgateway.TokenResponse{
		ID:           pair.Access.ID,
		TokenType:    "Bearer",
		AccessToken:  pair.Access.Token,
		RefreshToken: pair.Refresh.Token,
		ExpiresAt:    pair.Access.ExpiresAt,
	})
}

//	@Summary		Revoke token
//	@Description	Revoke access token with specified jti
//	@Security		ApiAuth
//	@Security		JWTAuth
//	@Tags			User, Auth
//	@Produce		json
//	@Param			jti	path	string	true	"JWT ID"
//	@Success		204	"No Content"
//	@Failure		401	{object}	smsgateway.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	smsgateway.ErrorResponse	"Forbidden"
//	@Failure		500	{object}	smsgateway.ErrorResponse	"Internal server error"
//	@Failure		501	{object}	smsgateway.ErrorResponse	"Not implemented"
//	@Router			/3rdparty/v1/auth/token/{jti} [delete]
//
// Revoke token.
func (h *AuthHandler) deleteToken(userID string, c *fiber.Ctx) error {
	jti := c.Params("jti")

	if err := h.jwtSvc.RevokeToken(c.Context(), userID, jti); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) errorHandler(c *fiber.Ctx) error {
	err := c.Next()
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, jwt.ErrInvalidParams):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())

	case errors.Is(err, jwt.ErrInvalidToken),
		errors.Is(err, jwt.ErrTokenRevoked),
		errors.Is(err, jwt.ErrInvalidTokenUse),
		errors.Is(err, jwt.ErrTokenReplay):
		return fiber.ErrUnauthorized

	case errors.Is(err, jwt.ErrInitFailed):
		fallthrough
	case errors.Is(err, jwt.ErrInvalidConfig):
		return fiber.NewError(
			fiber.StatusInternalServerError,
			"token service not configured, contact your administrator",
		)

	case errors.Is(err, jwt.ErrDisabled):
		return fiber.NewError(fiber.StatusNotImplemented, "token service disabled, contact your administrator")
	}

	return err //nolint:wrapcheck // passed through to fiber's error handler
}
