package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/usecase"
	"github.com/gin-gonic/gin"
)

// WebhookHandler ingests SePay bank-transfer callbacks and confirms deposits.
type WebhookHandler struct {
	uc        *usecase.WalletUseCase
	apiKey    string
	secretKey string
}

func NewWebhookHandler(uc *usecase.WalletUseCase, apiKey, secretKey string) *WebhookHandler {
	return &WebhookHandler{uc: uc, apiKey: apiKey, secretKey: secretKey}
}

type sepayPayload struct {
	Content        string  `json:"content"`
	TransferAmount float64 `json:"transferAmount"`
	ReferenceCode  string  `json:"referenceCode"`
	TransferType   string  `json:"transferType"`
	Description    string  `json:"description"`
}

// SepayCallback validates the API key / HMAC signature, then confirms the
// deposit referenced in the transfer memo.
func (h *WebhookHandler) SepayCallback(c *gin.Context) {
	if !h.validate(c) {
		response.Error(c, 401, "unauthorized webhook")
		return
	}
	var payload sepayPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	orderCode := extractOrderCode(payload.Content)
	if orderCode == "" {
		response.Error(c, 400, "order code not found in content")
		return
	}
	if err := h.uc.ConfirmDeposit(c.Request.Context(), orderCode); err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "deposit confirmed"})
}

// validate checks the X-API-Key header or an HMAC-SHA256 X-Signature.
// Fail-closed: rejects when no auth material is configured.
func (h *WebhookHandler) validate(c *gin.Context) bool {
	if key := c.GetHeader("X-API-Key"); key != "" && h.apiKey != "" && key == h.apiKey {
		return true
	}
	if sig := c.GetHeader("X-Signature"); sig != "" && h.secretKey != "" {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return false
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
		mac := hmac.New(sha256.New, []byte(h.secretKey))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(sig), []byte(expected))
	}
	return false
}

func extractOrderCode(content string) string {
	for _, word := range strings.Fields(content) {
		if strings.HasPrefix(word, "DEP-") {
			return word
		}
	}
	return ""
}
