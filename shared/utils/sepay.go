package utils

import "fmt"

func GenerateSepayQR(bankAccount, bankCode string, amount float64, description string) string {
	return fmt.Sprintf("https://qr.sepay.vn/img?acc=%s&bank=%s&amount=%.0f&des=%s",
		bankAccount, bankCode, amount, description)
}
