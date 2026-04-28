package handler

import (
	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates account and sets HttpOnly access + refresh cookies
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  service.RegisterReq  true  "Registration payload"
// @Success      201   {object}  map[string]interface{}  "user object"
// @Failure      400   {object}  map[string]interface{}
// @Router       /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req service.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	req.ClientIP = c.ClientIP()
	req.UserAgent = c.Request.UserAgent()

	user, access, refresh, err := h.auth.Register(req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	middleware.SetAuthCookies(c, access, refresh)
	response.Created(c, gin.H{"user": user})
}

// Login godoc
// @Summary      Login
// @Description  Authenticates user; sets cookies on success. Returns requires2FA flag if 2FA enabled.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  service.LoginReq  true  "Login payload"
// @Success      200   {object}  map[string]interface{}
// @Failure      401   {object}  map[string]interface{}
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	req.ClientIP = c.ClientIP()
	req.UserAgent = c.Request.UserAgent()
	req.AcceptLanguage = c.GetHeader("Accept-Language")

	result, err := h.auth.Login(req)
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}
	if result.Requires2FA {
		response.OK(c, gin.H{"requires2FA": true, "tempToken": result.TempToken})
		return
	}
	if result.RequiresStepUp {
		response.OK(c, gin.H{"requiresStepUp": true, "stepUpToken": result.StepUpToken})
		return
	}
	middleware.SetAuthCookies(c, result.AccessToken, result.RefreshToken)
	response.OK(c, gin.H{"user": result.User})
}

// StepUp godoc
// @Summary      Complete step-up (new-device) challenge
// @Description  Confirms new-device challenge; issues access + refresh cookies on success
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  object{token=string,code=string}  true  "Step-up payload"
// @Success      200   {object}  map[string]interface{}
// @Failure      401   {object}  map[string]interface{}
// @Router       /auth/step-up [post]
func (h *AuthHandler) StepUp(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := h.auth.CompleteStepUp(req.Token, req.Code, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}
	middleware.SetAuthCookies(c, result.AccessToken, result.RefreshToken)
	response.OK(c, gin.H{"user": result.User})
}

func (h *AuthHandler) Login2FA(c *gin.Context) {
	var req struct {
		TempToken string `json:"tempToken" binding:"required"`
		Code      string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	access, refresh, user, err := h.auth.Login2FA(req.TempToken, req.Code, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		response.Error(c, 401, err.Error())
		return
	}
	middleware.SetAuthCookies(c, access, refresh)
	response.OK(c, gin.H{"user": user})
}

// Profile godoc
// @Summary      Get current user profile
// @Tags         auth
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]interface{}
// @Router       /auth/profile [get]
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	user, err := h.auth.GetProfile(userID)
	if err != nil {
		response.Error(c, 404, "user not found")
		return
	}
	response.OK(c, user)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req struct {
		FullName string `json:"fullName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	user, err := h.auth.UpdateProfile(userID, req.FullName)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, user)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.auth.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	// Password change revoked all sessions — force the caller to re-login too.
	middleware.ClearAuthCookies(c)
	response.OK(c, gin.H{"message": "password changed; all sessions revoked"})
}

// RefreshAccessToken godoc
// @Summary      Refresh access token
// @Description  Rotates refresh token cookie and re-issues both cookies
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      401  {object}  map[string]interface{}
// @Router       /auth/refresh [post]
func (h *AuthHandler) RefreshAccessToken(c *gin.Context) {
	refreshToken, err := c.Cookie(middleware.RefreshCookieName)
	if err != nil || refreshToken == "" {
		response.Error(c, 401, "refresh token required")
		return
	}
	access, newRefresh, err := h.auth.RefreshToken(refreshToken, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		// Always clear cookies on refresh failure — reduces stale-token confusion.
		middleware.ClearAuthCookies(c)
		response.Error(c, 401, err.Error())
		return
	}
	middleware.SetAuthCookies(c, access, newRefresh)
	response.OK(c, gin.H{"refreshed": true})
}

// LogoutHandler godoc
// @Summary      Logout current session
// @Tags         auth
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  map[string]interface{}
// @Router       /auth/logout [post]
func (h *AuthHandler) LogoutHandler(c *gin.Context) {
	userID := middleware.GetUserID(c)
	email, _ := c.Get("email")
	emailStr, _ := email.(string)
	if cookie, err := c.Cookie(middleware.RefreshCookieName); err == nil && cookie != "" {
		_ = h.auth.Logout(cookie, userID, emailStr, c.ClientIP(), c.GetHeader("User-Agent"))
	}
	middleware.ClearAuthCookies(c)
	response.OK(c, gin.H{"message": "logged out"})
}

// LogoutAll revokes every active refresh token of the current user.
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID := middleware.GetUserID(c)
	_ = h.auth.LogoutAll(userID)
	middleware.ClearAuthCookies(c)
	response.OK(c, gin.H{"message": "all sessions revoked"})
}

func (h *AuthHandler) Enable2FA(c *gin.Context) {
	userID := middleware.GetUserID(c)
	_, qrURL, err := h.auth.Enable2FA(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"qrURL": qrURL})
}

func (h *AuthHandler) Verify2FA(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.auth.Verify2FA(userID, req.Code); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "2FA enabled"})
}

func (h *AuthHandler) Disable2FA(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.auth.Disable2FA(userID, req.Code); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "2FA disabled"})
}

// WSToken returns a short-lived access token to attach as ?token= on the WS handshake.
// The HttpOnly access cookie is invisible to JS, so this endpoint is the bridge.
func (h *AuthHandler) WSToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	token, err := h.auth.GenerateWSToken(userID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"token": token})
}
