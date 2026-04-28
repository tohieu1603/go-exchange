package redisutil

import "github.com/redis/go-redis/v9"

// BalanceDeduct atomically checks and deducts balance in Redis.
// KEYS[1] = wallet key (e.g., "bal:5:USDT")
// ARGV[1] = amount to deduct (positive number)
// Returns: new balance on success, -1 if insufficient, -2 if key not found
var BalanceDeduct = redis.NewScript(`
	local bal = redis.call('GET', KEYS[1])
	if bal == false then return -2 end
	bal = tonumber(bal)
	local amount = tonumber(ARGV[1])
	if bal < amount then return -1 end
	local newBal = bal - amount
	redis.call('SET', KEYS[1], string.format("%.10f", newBal))
	return tostring(newBal)
`)

// BalanceCredit atomically credits balance in Redis.
// KEYS[1] = wallet key
// ARGV[1] = amount to credit
// Returns: new balance
var BalanceCredit = redis.NewScript(`
	local bal = redis.call('GET', KEYS[1])
	if bal == false then
		redis.call('SET', KEYS[1], string.format("%.10f", tonumber(ARGV[1])))
		return ARGV[1]
	end
	local newBal = tonumber(bal) + tonumber(ARGV[1])
	redis.call('SET', KEYS[1], string.format("%.10f", newBal))
	return tostring(newBal)
`)

// BalanceLock atomically checks available (balance - locked) and increases locked.
// KEYS[1] = balance key (e.g., "bal:5:USDT")
// KEYS[2] = locked key (e.g., "locked:5:USDT")
// ARGV[1] = amount to lock
// Returns: 1 on success, -1 if insufficient available
var BalanceLock = redis.NewScript(`
	local bal = tonumber(redis.call('GET', KEYS[1]) or '0')
	local locked = tonumber(redis.call('GET', KEYS[2]) or '0')
	local amount = tonumber(ARGV[1])
	if (bal - locked) < amount then return -1 end
	redis.call('SET', KEYS[2], string.format("%.10f", locked + amount))
	return 1
`)

// BalanceUnlock atomically decreases locked balance.
// KEYS[1] = locked key
// ARGV[1] = amount to unlock
// Returns: new locked balance
var BalanceUnlock = redis.NewScript(`
	local locked = tonumber(redis.call('GET', KEYS[1]) or '0')
	local amount = tonumber(ARGV[1])
	local newLocked = locked - amount
	if newLocked < 0 then newLocked = 0 end
	redis.call('SET', KEYS[1], string.format("%.10f", newLocked))
	return tostring(newLocked)
`)

// BalanceTransfer atomically deducts from one wallet and credits another.
// KEYS[1] = source balance key
// KEYS[2] = dest balance key
// ARGV[1] = amount
// Returns: 1 on success, -1 if insufficient source balance
var BalanceTransfer = redis.NewScript(`
	local srcBal = tonumber(redis.call('GET', KEYS[1]) or '0')
	local amount = tonumber(ARGV[1])
	if srcBal < amount then return -1 end
	redis.call('SET', KEYS[1], string.format("%.10f", srcBal - amount))
	local dstBal = tonumber(redis.call('GET', KEYS[2]) or '0')
	redis.call('SET', KEYS[2], string.format("%.10f", dstBal + amount))
	return 1
`)

// RateLimit implements sliding window rate limiting.
// KEYS[1] = rate limit key (e.g., "rl:login:192.168.1.1")
// ARGV[1] = window size in seconds
// ARGV[2] = max requests allowed
// ARGV[3] = current timestamp (unix ms)
// Returns: 1 if allowed, 0 if rate limited
var RateLimit = redis.NewScript(`
	local key = KEYS[1]
	local window = tonumber(ARGV[1]) * 1000
	local maxReq = tonumber(ARGV[2])
	local now = tonumber(ARGV[3])
	local clearBefore = now - window
	redis.call('ZREMRANGEBYSCORE', key, 0, clearBefore)
	local count = redis.call('ZCARD', key)
	if count >= maxReq then return 0 end
	redis.call('ZADD', key, now, now .. ':' .. math.random(1000000))
	redis.call('PEXPIRE', key, window)
	return 1
`)

// PriceUpdate atomically updates price + ticker data for a pair.
// KEYS[1] = price key (e.g., "price:BTC_USDT")
// KEYS[2] = ticker key (e.g., "ticker:BTC_USDT")
// ARGV[1] = price
// ARGV[2] = change24h
// ARGV[3] = volume24h
// ARGV[4] = TTL in seconds
var PriceUpdate = redis.NewScript(`
	local ttl = tonumber(ARGV[4])
	redis.call('SET', KEYS[1], ARGV[1], 'EX', ttl)
	redis.call('HMSET', KEYS[2], 'price', ARGV[1], 'change24h', ARGV[2], 'volume24h', ARGV[3])
	redis.call('EXPIRE', KEYS[2], ttl)
	return 1
`)
