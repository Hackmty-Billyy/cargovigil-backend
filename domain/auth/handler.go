package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes wires the auth endpoints. loginLimiter is applied only to
// the two credential-guessing surfaces (password + TOTP check); authMiddleware
// guards everything that requires an already-authenticated session.
func (h *Handler) RegisterRoutes(router fiber.Router, authMiddleware, loginLimiter fiber.Handler) {
	router.Post("/register", h.Register)
	router.Post("/login", loginLimiter, h.Login)
	router.Post("/login/verify-totp", loginLimiter, h.VerifyTOTP)
	router.Post("/refresh", h.Refresh)

	protected := router.Group("", authMiddleware)
	protected.Get("/me", h.GetMe)
	protected.Post("/totp/enroll", h.EnrollTOTP)
	protected.Post("/totp/confirm", h.ConfirmTOTP)
	protected.Post("/logout", h.Logout)
	protected.Post("/logout/all", h.LogoutAll)
	protected.Post("/recovery-codes/regenerate", h.RegenerateRecoveryCodes)
}

func sessionMetaFromCtx(c *fiber.Ctx) SessionMeta {
	return SessionMeta{UserAgent: c.Get("User-Agent"), IP: c.IP()}
}

func userIDFromCtx(c *fiber.Ctx) string {
	id, _ := c.Locals("user_id").(string)
	return id
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}
	if req.Role == "" {
		req.Role = "operations"
	}
	u, err := h.svc.Register(c.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":    u.ID,
		"email": u.Email,
		"role":  req.Role,
	})
}

func (h *Handler) GetMe(c *fiber.Ctx) error {
	profile, err := h.svc.GetProfile(c.Context(), userIDFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(profile)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	result, err := h.svc.Login(c.Context(), req.Email, req.Password, sessionMetaFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(result)
}

type verifyTOTPRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

func (h *Handler) VerifyTOTP(c *fiber.Ctx) error {
	var req verifyTOTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	result, err := h.svc.VerifyTOTP(c.Context(), req.MFAToken, req.Code, sessionMetaFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(result)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	result, err := h.svc.RefreshSession(c.Context(), req.RefreshToken, sessionMetaFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) EnrollTOTP(c *fiber.Ctx) error {
	secret, url, err := h.svc.EnrollTOTP(c.Context(), userIDFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(fiber.Map{"secret": secret, "otpauth_url": url})
}

type confirmTOTPRequest struct {
	Code string `json:"code"`
}

func (h *Handler) ConfirmTOTP(c *fiber.Ctx) error {
	var req confirmTOTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	codes, err := h.svc.ConfirmTOTP(c.Context(), userIDFromCtx(c), req.Code)
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(fiber.Map{"recovery_codes": codes})
}

func (h *Handler) RegenerateRecoveryCodes(c *fiber.Ctx) error {
	codes, err := h.svc.RegenerateRecoveryCodes(c.Context(), userIDFromCtx(c))
	if err != nil {
		return handleAuthError(c, err)
	}
	return c.JSON(fiber.Map{"recovery_codes": codes})
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.svc.Logout(c.Context(), req.RefreshToken); err != nil {
		return handleAuthError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) LogoutAll(c *fiber.Ctx) error {
	if err := h.svc.LogoutAll(c.Context(), userIDFromCtx(c)); err != nil {
		return handleAuthError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func handleAuthError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUserInactive),
		errors.Is(err, ErrInvalidTOTPCode), errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrInvalidRefreshToken), errors.Is(err, ErrRefreshTokenExpired):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	case errors.Is(err, ErrSessionCompromised):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "session compromised, please log in again"})
	case errors.Is(err, ErrTOTPAlreadyEnabled), errors.Is(err, ErrTOTPNotEnrolled):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
}
