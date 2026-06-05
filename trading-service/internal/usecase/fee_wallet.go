package usecase

import "sync/atomic"

// platformFeeUserID is the user ID of the platform fee-collection wallet,
// resolved once at startup (by infrastructure/feewallet) and read on every
// trade. Stored atomically for a lock-free hot-path read.
var platformFeeUserID atomic.Uint64

// PlatformFeeUserID returns the fee wallet user ID (0 if unresolved — callers
// must skip fee crediting in that case).
func PlatformFeeUserID() uint { return uint(platformFeeUserID.Load()) }

// SetPlatformFeeUserID is called by the infrastructure resolver once the ID is
// known.
func SetPlatformFeeUserID(id uint) { platformFeeUserID.Store(uint64(id)) }
