package service

import "github.com/pquerna/otp/totp"

type TOTPService struct{}

func NewTOTPService() *TOTPService {
	return &TOTPService{}
}

// GenerateSecret creates a new TOTP secret for a user.
func (s *TOTPService) GenerateSecret(email string) (secret string, qrURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "CryptoExchange",
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode verifies a 6-digit TOTP code against the secret.
func (s *TOTPService) ValidateCode(secret, code string) bool {
	return totp.Validate(code, secret)
}
