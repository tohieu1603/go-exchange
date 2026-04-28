package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// DeviceFingerprint returns a stable 16-char hex hash derived from
// (User-Agent, Accept-Language, IP /24 prefix). Used by audit log to detect
// new-device login attempts without storing PII.
//
// IP /24 (e.g. 1.2.3.0) is intentional: same residential ISP, different
// rotation IPs, same device. Wider prefix lets us tolerate network changes
// while still flagging country/ASN-level moves.
//
// This is a heuristic — not strong identification. For production, augment
// with browser FingerprintJS or push-notification confirmation on new device.
func DeviceFingerprint(ip, userAgent, acceptLanguage string) string {
	h := sha256.New()
	h.Write([]byte(ipv4Prefix24(ip)))
	h.Write([]byte{0})
	h.Write([]byte(userAgent))
	h.Write([]byte{0})
	h.Write([]byte(acceptLanguage))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ipv4Prefix24 returns the /24 of an IPv4 (e.g. "203.0.113.42" → "203.0.113").
// IPv6 and unrecognized inputs pass through unchanged. IPv6 has its own
// privacy norms (interface-id rotation), so a full match is acceptable signal.
func ipv4Prefix24(ip string) string {
	dots := 0
	for i := 0; i < len(ip); i++ {
		if ip[i] == '.' {
			dots++
			if dots == 3 {
				return ip[:i]
			}
		}
		if ip[i] == ':' { // IPv6 → leave as-is
			return ip
		}
	}
	return ip
}
