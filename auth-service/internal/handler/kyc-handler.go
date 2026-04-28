package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cryptox/auth-service/internal/service"
	"github.com/cryptox/shared/middleware"
	"github.com/cryptox/shared/response"
	"github.com/gin-gonic/gin"
)

const maxFileSize = 5 << 20 // 5MB

var allowedExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// KYCHandler handles all KYC-related HTTP endpoints.
type KYCHandler struct {
	svc *service.KYCService
}

func NewKYCHandler(svc *service.KYCService) *KYCHandler {
	return &KYCHandler{svc: svc}
}

// POST /api/kyc/email/send — dispatches a verification code to the user's email.
// The code is NEVER returned in the response (sent via SMTP / logged in dev).
func (h *KYCHandler) SendVerifyEmail(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.svc.SendVerifyEmail(userID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "verification code sent to your email"})
}

// POST /api/kyc/email/verify
func (h *KYCHandler) VerifyEmail(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.svc.VerifyEmail(userID, req.Code); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "email verified successfully"})
}

// POST /api/kyc/profile
func (h *KYCHandler) SubmitProfile(c *gin.Context) {
	var req service.SubmitProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if err := h.svc.SubmitProfile(userID, req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "profile submitted successfully"})
}

// POST /api/kyc/document — multipart/form-data: type + file
func (h *KYCHandler) UploadDocument(c *gin.Context) {
	userID := middleware.GetUserID(c)
	docType := strings.ToUpper(strings.TrimSpace(c.PostForm("type")))
	if docType == "" {
		response.Error(c, 400, "field 'type' is required")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "field 'file' is required")
		return
	}
	defer file.Close()

	if header.Size > maxFileSize {
		response.Error(c, 400, "file size must not exceed 5MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExts[ext] {
		response.Error(c, 400, "only jpg, jpeg, png files are allowed")
		return
	}

	uploadDir := fmt.Sprintf("./uploads/kyc/%d", userID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		response.Error(c, 500, "failed to create upload directory")
		return
	}

	filename := fmt.Sprintf("%s%s", strings.ToLower(docType), ext)
	savePath := filepath.Join(uploadDir, filename)
	if err := c.SaveUploadedFile(header, savePath); err != nil {
		response.Error(c, 500, "failed to save file")
		return
	}

	if err := h.svc.UploadDocument(userID, docType, savePath); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "document uploaded successfully", "path": savePath})
}

// GET /api/kyc/status
func (h *KYCHandler) GetStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)
	status, err := h.svc.GetKYCStatus(userID)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, status)
}

// POST /api/admin/kyc/:userId/approve
func (h *KYCHandler) ApproveKYC(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)

	if err := h.svc.ApproveKYC(userID, req.Note); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "KYC approved"})
}

// POST /api/admin/kyc/:userId/reject
func (h *KYCHandler) RejectKYC(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&req)

	if err := h.svc.RejectKYC(userID, req.Note); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "KYC rejected"})
}

// GET /api/admin/kyc/pending
func (h *KYCHandler) ListPending(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	users, total, err := h.svc.GetPendingUsers(page, size)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"users": users, "total": total, "page": page, "size": size})
}

func parseUserIDParam(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid userId param")
	}
	return uint(id), nil
}
