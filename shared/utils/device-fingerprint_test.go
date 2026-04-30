package utils

import "testing"

func TestDeviceFingerprint_StableAcrossSameDevice(t *testing.T) {
	a := DeviceFingerprint("203.0.113.42", "Chrome/120", "en-US")
	b := DeviceFingerprint("203.0.113.42", "Chrome/120", "en-US")
	if a != b {
		t.Fatal("same inputs should yield same fingerprint")
	}
	if len(a) != 16 {
		t.Errorf("expected 16-char fingerprint, got %d", len(a))
	}
}

func TestDeviceFingerprint_TolerantToIPRotationInSameSubnet(t *testing.T) {
	// /24 design: 203.0.113.42 and 203.0.113.99 should hash the same so
	// residential ISP IP rotation doesn't false-positive new-device.
	a := DeviceFingerprint("203.0.113.42", "Chrome/120", "en-US")
	b := DeviceFingerprint("203.0.113.99", "Chrome/120", "en-US")
	if a != b {
		t.Fatal("/24 prefix should produce same fingerprint for same subnet")
	}
}

func TestDeviceFingerprint_DiffersOnDifferentSubnet(t *testing.T) {
	a := DeviceFingerprint("203.0.113.42", "Chrome/120", "en-US")
	b := DeviceFingerprint("198.51.100.42", "Chrome/120", "en-US")
	if a == b {
		t.Fatal("different /24 subnets should differ")
	}
}

func TestDeviceFingerprint_DiffersOnUserAgent(t *testing.T) {
	a := DeviceFingerprint("203.0.113.42", "Chrome/120", "en-US")
	b := DeviceFingerprint("203.0.113.42", "Firefox/121", "en-US")
	if a == b {
		t.Fatal("UA change should change fingerprint")
	}
}

func TestDeviceFingerprint_IPv6PassesThrough(t *testing.T) {
	// Sanity: IPv6 input must not panic; the function leaves IPv6 unchanged
	// (privacy via interface-id rotation handled separately).
	a := DeviceFingerprint("2001:db8::1", "UA", "en")
	b := DeviceFingerprint("2001:db8::1", "UA", "en")
	if a != b {
		t.Fatal("IPv6 inputs should be stable")
	}
	if len(a) != 16 {
		t.Errorf("expected 16-char output, got %d", len(a))
	}
}
