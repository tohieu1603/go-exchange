package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"github.com/cryptox/shared/response"
	"github.com/cryptox/wallet-service/internal/service"
	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	wallet    *service.WalletService
	apiKey    string
	secretKey string
}

func NewWebhookHandler(wallet *service.WalletService, apiKey, secretKey string) *WebhookHandler {
	return &WebhookHandler{wallet: wallet, apiKey: apiKey, secretKey: secretKey}
}

type SepayPayload struct {
	Content        string  `json:"content"`
	TransferAmount float64 `json:"transferAmount"`
	ReferenceCode  string  `json:"referenceCode"`
	TransferType   string  `json:"transferType"`
	Description    string  `json:"description"`
}

// SepayCallback handles SePay bank transfer webhook notifications.
// Validates API key header or HMAC signature before processing.
func (h *WebhookHandler) SepayCallback(c *gin.Context) {
	if !h.validateWebhook(c) {
		response.Error(c, 401, "unauthorized webhook")
		return
	}

	var payload SepayPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	orderCode := extractOrderCode(payload.Content)
	if orderCode == "" {
		response.Error(c, 400, "order code not found in content")
		return
	}

	if err := h.wallet.ConfirmDeposit(orderCode); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	response.OK(c, gin.H{"message": "deposit confirmed"})
}

// validateWebhook checks API key header or HMAC-SHA256 signature
func (h *WebhookHandler) validateWebhook(c *gin.Context) bool {
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" && h.apiKey != "" && apiKey == h.apiKey {
		return true
	}

	sig := c.GetHeader("X-Signature")
	if sig != "" && h.secretKey != "" {
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

	// Fail-closed: reject if no auth keys configured
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
