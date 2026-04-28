# Refactor: Tách `shared/model` thành per-service models

## Mục tiêu

Loại bỏ violation của database-per-service principle. Mỗi vi dịch vụ sở hữu models của riêng mình, không service nào import struct domain của service khác.

## Nguyên tắc giữ lại trong `shared/`

| Có giữ | Lý do |
|--------|-------|
| `shared/eventbus/events.go` (DTO) | Cross-service event payload — đã là DTO, không phải GORM model |
| `shared/proto/*.proto` | gRPC contract — versioned interface |
| `shared/middleware/`, `shared/utils/`, `shared/redisutil/`, `shared/mailer/`, `shared/ws/` | Cross-cutting concerns |
| `shared/types/` (mới) | Enum constants (OrderSide, OrderType, RoleSystem...) + `Coin`/`DefaultCoins` |

## Map ownership

| Service | Models thuộc về |
|---------|----------------|
| auth-service | user, kyc-profile, kyc-document, fraud-log, bonus-promotion, user-bonus, platform-settings, refresh-token, api-key, fee-tier, user-volume30d, referral-code, referral, referral-commission |
| wallet-service | wallet, deposit, withdrawal |
| market-service | candle, user-trade-pair |
| trading-service | order, trade |
| futures-service | futures-position, funding-rate, funding-payment |
| notification-service | notification |

## Thay đổi cần làm cho mỗi service

1. Tạo `<service>/internal/model/*.go` (move file từ `shared/model/`)
2. Đổi `package model` (giữ nguyên)
3. Update mọi import trong service đó: `github.com/cryptox/shared/model` → `github.com/cryptox/<service>/internal/model`
4. Service KHÁC vẫn cần một số shared concepts → di chuyển sang `shared/types/`

## Cross-service dependencies hiện tại

| Service A → cần model của B | Cách giải quyết |
|----------------------------|----------------|
| auth-service.SeedAdmin → tạo `Wallet` row trong pg-auth | Copy `Wallet` thành minimal struct trong auth-service (auth-service đã có wallets table riêng cho seed) hoặc tốt hơn: bỏ logic seed wallet trong auth, để wallet-service consume `user.registered` event tạo |
| futures.Liquidation → `Wallet` | Đã dùng gRPC `walletClient.Credit` — không cần model |
| auth-service → `Coin` (DefaultCoins) | Move `coin.go` sang `shared/types/coin.go` |
| trading-service → `Coin` (DefaultCoins) | Tương tự |
| auth-service → `UserTradePair` | Đây là model của market-service, auto-migrate sai vị trí. Move sang market-service. Auth không cần nó |

## Bảo mật cải tiến (đính kèm)

Vì đang refactor "production-grade", áp dụng các bảo mật:

1. **Argon2id** thay bcrypt cho password hashing (RFC 9106 — chống GPU brute force tốt hơn)
2. **Audit log** mỗi action sensitive: login, password change, withdrawal, API key create
3. **Constant-time email lookup** trong Login để chống timing attack (đã có một phần)
4. **Strict IP whitelist** cho API key withdraw (yêu cầu IPWhitelist non-empty nếu permission có "withdraw")
5. **Rotate jwtSecret** support — JWT có `kid` field, server hold N gen
6. **PII redaction** trong log — không log email/IP đầy đủ
7. **Withdraw 2FA-required** — nếu user có 2FA, withdraw cần TOTP code

## Thứ tự thực hiện

### Phase 1: Setup `shared/types/`
- Tạo `shared/types/coin.go` (move từ `shared/model/coin.go`)
- Tạo `shared/types/enum.go` (OrderSide, OrderType, KYCStatus, PositionSide, OrderStatus, ...)
- Tạo `shared/types/system.go` (RoleSystem, SystemEmailFeeWallet)
- Update toàn bộ services dùng `model.DefaultCoins` → `types.DefaultCoins`

### Phase 2: Move models per-service (parallel-friendly)
Mỗi service làm độc lập:
- Tạo `<svc>/internal/model/`
- Move file
- Update import
- Build verify

### Phase 3: Xóa `shared/model/`
- Xóa các file đã move
- Build toàn bộ verify
- `shared/model/` chỉ còn (nếu cần): `coin.go` (legacy aliases) hoặc xóa hẳn

### Phase 4: Bảo mật cải tiến
- Argon2id password hash + migration mật khẩu cũ on-login
- Audit log table + service
- Withdraw 2FA gate
- Strict IP whitelist for withdraw API keys

### Phase 5: Tests
- Unit test cho matching engine (price-time priority, rollback)
- Integration test login → refresh rotate → replay detection
- API key HMAC sign verify

## Số file ước tính

- 17 model files move
- ~80 import updates across services
- 4-5 new shared/types files
- 6-8 new security files (argon2, audit log, withdraw 2FA, etc.)

## Câu hỏi mở

1. **Wallet** model: nên ở `wallet-service` (đúng ownership) nhưng `auth-service.SeedAdmin` đang trực tiếp tạo wallets cho admin. Có 2 cách:
   - (A) Bỏ logic seed wallet trong auth-service, để wallet-service tạo qua `user.registered` event
   - (B) auth-service giữ minimal `Wallet` struct riêng để seed (không lý tưởng)
2. **Argon2id migration**: lưu `password_hash_v2` mới song song với `password_hash` cũ, on-login nếu match cũ → rehash sang mới?
3. Bạn muốn ưu tiên phase nào trước?
