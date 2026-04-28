// Package types holds cross-service value types and constants.
//
// Allowed contents:
//   - Reference data shared across services (Coin metadata, DefaultCoins)
//   - String enum constants (OrderSide, KYCStatus, ...)
//   - System account identifiers
//
// NOT allowed:
//   - GORM models owned by a specific service (User, Wallet, Order, ...)
//     Those live in <service>/internal/model/
//   - Service-specific business logic
package types

// Coin is reference metadata for a tradeable asset. The persisted table is
// owned by auth-service (admin manages active/inactive coins).
type Coin struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Symbol      string `gorm:"uniqueIndex;not null" json:"symbol"`
	Name        string `gorm:"not null" json:"name"`
	CoinGeckoID string `json:"coinGeckoId"`
	BybitSymbol string `json:"bybitSymbol"` // override (e.g. "XAUTUSDT")
	IconURL     string `json:"iconUrl"`
	IsActive    bool   `gorm:"default:true" json:"isActive"`
	SortOrder   int    `gorm:"default:0" json:"sortOrder"`
	AssetType   string `gorm:"default:'crypto'" json:"assetType"` // crypto, forex, commodity
}

// GetBybitSymbol returns the explicit override or the default Symbol+USDT.
func (c Coin) GetBybitSymbol() string {
	if c.BybitSymbol != "" {
		return c.BybitSymbol
	}
	return c.Symbol + "USDT"
}

// DefaultCoins is the seed list — authoritative source for pair definitions
// across services. Services iterate this list to register pairs (e.g. order
// books, candle aggregators). Auth-service additionally persists this set.
var DefaultCoins = []Coin{
	{Symbol: "BTC", Name: "Bitcoin", CoinGeckoID: "bitcoin", SortOrder: 1, IsActive: true},
	{Symbol: "ETH", Name: "Ethereum", CoinGeckoID: "ethereum", SortOrder: 2, IsActive: true},
	{Symbol: "BNB", Name: "BNB", CoinGeckoID: "binancecoin", SortOrder: 3, IsActive: true},
	{Symbol: "SOL", Name: "Solana", CoinGeckoID: "solana", SortOrder: 4, IsActive: true},
	{Symbol: "DOGE", Name: "Dogecoin", CoinGeckoID: "dogecoin", SortOrder: 5, IsActive: true},
	{Symbol: "XRP", Name: "XRP", CoinGeckoID: "ripple", SortOrder: 6, IsActive: true},
	{Symbol: "ADA", Name: "Cardano", CoinGeckoID: "cardano", SortOrder: 7, IsActive: true},
	{Symbol: "DOT", Name: "Polkadot", CoinGeckoID: "polkadot", SortOrder: 8, IsActive: true},
	{Symbol: "MATIC", Name: "Polygon", CoinGeckoID: "matic-network", SortOrder: 9, IsActive: true},
	{Symbol: "AVAX", Name: "Avalanche", CoinGeckoID: "avalanche-2", SortOrder: 10, IsActive: true},
	{Symbol: "LINK", Name: "Chainlink", CoinGeckoID: "chainlink", SortOrder: 11, IsActive: true},
	{Symbol: "UNI", Name: "Uniswap", CoinGeckoID: "uniswap", SortOrder: 12, IsActive: true},
	{Symbol: "ATOM", Name: "Cosmos", CoinGeckoID: "cosmos", SortOrder: 13, IsActive: true},
	{Symbol: "FIL", Name: "Filecoin", CoinGeckoID: "filecoin", SortOrder: 14, IsActive: true},
	{Symbol: "LTC", Name: "Litecoin", CoinGeckoID: "litecoin", SortOrder: 15, IsActive: true},
	{Symbol: "NEAR", Name: "NEAR Protocol", CoinGeckoID: "near", SortOrder: 16, IsActive: true},
	{Symbol: "APT", Name: "Aptos", CoinGeckoID: "aptos", SortOrder: 17, IsActive: true},
	{Symbol: "ARB", Name: "Arbitrum", CoinGeckoID: "arbitrum", SortOrder: 18, IsActive: true},
	{Symbol: "OP", Name: "Optimism", CoinGeckoID: "optimism", SortOrder: 19, IsActive: true},
	{Symbol: "SHIB", Name: "Shiba Inu", CoinGeckoID: "shiba-inu", SortOrder: 20, IsActive: true},
	{Symbol: "TRX", Name: "TRON", CoinGeckoID: "tron", SortOrder: 21, IsActive: true},
	{Symbol: "ICP", Name: "Internet Computer", CoinGeckoID: "internet-computer", SortOrder: 22, IsActive: true},
	{Symbol: "HBAR", Name: "Hedera", CoinGeckoID: "hedera-hashgraph", SortOrder: 23, IsActive: true},
	{Symbol: "VET", Name: "VeChain", CoinGeckoID: "vechain", SortOrder: 24, IsActive: true},
	{Symbol: "FTM", Name: "Fantom", CoinGeckoID: "fantom", SortOrder: 25, IsActive: true},
	{Symbol: "ALGO", Name: "Algorand", CoinGeckoID: "algorand", SortOrder: 26, IsActive: true},
	{Symbol: "SAND", Name: "The Sandbox", CoinGeckoID: "the-sandbox", SortOrder: 27, IsActive: true},
	{Symbol: "MANA", Name: "Decentraland", CoinGeckoID: "decentraland", SortOrder: 28, IsActive: true},
	{Symbol: "AAVE", Name: "Aave", CoinGeckoID: "aave", SortOrder: 29, IsActive: true},
	{Symbol: "GRT", Name: "The Graph", CoinGeckoID: "the-graph", SortOrder: 30, IsActive: true},
	{Symbol: "MKR", Name: "Maker", CoinGeckoID: "maker", SortOrder: 31, IsActive: true},
	{Symbol: "SNX", Name: "Synthetix", CoinGeckoID: "synthetix-network-token", SortOrder: 32, IsActive: true},
	{Symbol: "CRV", Name: "Curve DAO", CoinGeckoID: "curve-dao-token", SortOrder: 33, IsActive: true},
	{Symbol: "LDO", Name: "Lido DAO", CoinGeckoID: "lido-dao", SortOrder: 34, IsActive: true},
	{Symbol: "IMX", Name: "Immutable X", CoinGeckoID: "immutable-x", SortOrder: 35, IsActive: true},
	{Symbol: "INJ", Name: "Injective", CoinGeckoID: "injective-protocol", SortOrder: 36, IsActive: true},
	{Symbol: "SUI", Name: "Sui", CoinGeckoID: "sui", SortOrder: 37, IsActive: true},
	{Symbol: "SEI", Name: "Sei", CoinGeckoID: "sei-network", SortOrder: 38, IsActive: true},
	{Symbol: "TIA", Name: "Celestia", CoinGeckoID: "celestia", SortOrder: 39, IsActive: true},
	{Symbol: "PEPE", Name: "Pepe", CoinGeckoID: "pepe", SortOrder: 40, IsActive: true},
	{Symbol: "FLOKI", Name: "Floki", CoinGeckoID: "floki", SortOrder: 41, IsActive: true},
	{Symbol: "RENDER", Name: "Render", CoinGeckoID: "render-token", SortOrder: 42, IsActive: true},
	{Symbol: "FET", Name: "Fetch.ai", CoinGeckoID: "fetch-ai", SortOrder: 43, IsActive: true},
	{Symbol: "JASMY", Name: "JasmyCoin", CoinGeckoID: "jasmycoin", SortOrder: 44, IsActive: true},
	{Symbol: "CHZ", Name: "Chiliz", CoinGeckoID: "chiliz", SortOrder: 45, IsActive: true},
	{Symbol: "ENS", Name: "Ethereum Name Service", CoinGeckoID: "ethereum-name-service", SortOrder: 46, IsActive: true},
	{Symbol: "CAKE", Name: "PancakeSwap", CoinGeckoID: "pancakeswap-token", SortOrder: 47, IsActive: true},
	{Symbol: "XLM", Name: "Stellar", CoinGeckoID: "stellar", SortOrder: 48, IsActive: true},
	{Symbol: "ETC", Name: "Ethereum Classic", CoinGeckoID: "ethereum-classic", SortOrder: 49, IsActive: true},
	{Symbol: "THETA", Name: "Theta Network", CoinGeckoID: "theta-token", SortOrder: 50, IsActive: true},

	// Forex & Commodities (via tokenized proxies on CoinGecko)
	{Symbol: "XAU", Name: "Gold (XAUUSD)", CoinGeckoID: "tether-gold", BybitSymbol: "XAUTUSDT", SortOrder: 51, IsActive: true, AssetType: "commodity"},
	{Symbol: "PAXG", Name: "PAX Gold", CoinGeckoID: "paxos-gold", BybitSymbol: "PAXGUSDT", SortOrder: 52, IsActive: true, AssetType: "commodity"},
	{Symbol: "EUR", Name: "Euro (EURUSD)", CoinGeckoID: "stasis-eurs", BybitSymbol: "EURUSDT", SortOrder: 53, IsActive: true, AssetType: "forex"},
	{Symbol: "GBP", Name: "Pound (GBPUSD)", CoinGeckoID: "poundtoken", BybitSymbol: "GBPUSDT", SortOrder: 54, IsActive: true, AssetType: "forex"},
	{Symbol: "JPY", Name: "Yen (USDJPY)", CoinGeckoID: "jpyc", BybitSymbol: "JPYUSDT", SortOrder: 55, IsActive: true, AssetType: "forex"},
}
