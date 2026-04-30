package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// truncateToInterval is the heart of the candle aggregator — every tick is
// bucketed into a candle by this function. Bugs here cause OHLCV data to
// straddle the wrong bucket, breaking historical charts.

func TestTruncateToInterval_MinuteAlignment(t *testing.T) {
	// 13:34:42.500 → 1m bucket starts at 13:34:00.
	in := time.Date(2026, 5, 1, 13, 34, 42, 500_000_000, time.UTC)
	got := truncateToInterval(in, "1m")
	assert.Equal(t, time.Date(2026, 5, 1, 13, 34, 0, 0, time.UTC), got)
}

func TestTruncateToInterval_MultiMinuteBuckets(t *testing.T) {
	// 13:07 → 5m bucket at 13:05; 15m bucket at 13:00; 30m bucket at 13:00.
	in := time.Date(2026, 5, 1, 13, 7, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"5m":  time.Date(2026, 5, 1, 13, 5, 0, 0, time.UTC),
		"15m": time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
		"30m": time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
	}
	for interval, want := range cases {
		t.Run(interval, func(t *testing.T) {
			assert.Equal(t, want, truncateToInterval(in, interval))
		})
	}
}

func TestTruncateToInterval_HourBuckets(t *testing.T) {
	in := time.Date(2026, 5, 1, 13, 45, 12, 0, time.UTC)
	cases := map[string]time.Time{
		"1h":  time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
		"2h":  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		"4h":  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		"6h":  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		"12h": time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	}
	for interval, want := range cases {
		t.Run(interval, func(t *testing.T) {
			assert.Equal(t, want, truncateToInterval(in, interval))
		})
	}
}

func TestTruncateToInterval_DayResetsTime(t *testing.T) {
	in := time.Date(2026, 5, 1, 23, 59, 59, 999_999_999, time.UTC)
	got := truncateToInterval(in, "1D")
	assert.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), got)
}

func TestTruncateToInterval_WeekStartsMonday(t *testing.T) {
	// Friday May 1, 2026 → ISO Monday is Apr 27.
	fri := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	got := truncateToInterval(fri, "1W")
	assert.Equal(t, time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC), got)

	// Sunday must not collapse to next-week's Monday — it's the *prior* Monday.
	// Sunday Apr 26 should map to Mon Apr 20.
	sun := time.Date(2026, 4, 26, 23, 0, 0, 0, time.UTC)
	got = truncateToInterval(sun, "1W")
	assert.Equal(t, time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC), got)
}

func TestTruncateToInterval_MonthFirstDayMidnight(t *testing.T) {
	in := time.Date(2026, 5, 17, 13, 45, 0, 0, time.UTC)
	got := truncateToInterval(in, "1M")
	assert.Equal(t, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), got)
}

func TestTruncateToInterval_UnknownFallsBackToHour(t *testing.T) {
	// Defensive: an unknown interval string should not panic; falls back to 1h.
	in := time.Date(2026, 5, 1, 13, 45, 12, 0, time.UTC)
	got := truncateToInterval(in, "weird-interval")
	assert.Equal(t, time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC), got)
}
