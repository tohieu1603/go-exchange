package types

// SystemEmailFeeWallet identifies the platform fee-collection account.
// Auth-service seeds this user; trading-service resolves its ID at startup
// via the Redis key RedisKeyFeeWalletID and credits trading fees there.
const (
	SystemEmailFeeWallet = "fee@system.local"
	SystemNameFeeWallet  = "Platform Fee Wallet"

	// Redis key holding the fee wallet's user ID. Auth publishes; trading reads.
	RedisKeyFeeWalletID = "system:fee_wallet_id"
)
