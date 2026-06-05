package domain

import (
	"testing"
	"time"
)

func TestLiveCandle_AccumulatesWithinBar(t *testing.T) {
	open := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	lc := NewLiveCandle(100, 5, open)

	if c, rolled := lc.Apply(110, 2, open); rolled || c != nil {
		t.Fatal("same boundary must not roll")
	}
	if c, rolled := lc.Apply(90, 3, open); rolled || c != nil {
		t.Fatal("same boundary must not roll")
	}
	// O=100 H=110 L=90 C=90 V=5+2+3=10
	if lc.Open != 100 || lc.High != 110 || lc.Low != 90 || lc.Close != 90 || lc.Volume != 10 {
		t.Fatalf("OHLCV wrong: %+v", lc)
	}
}

func TestLiveCandle_RollsToNewBar(t *testing.T) {
	open := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	next := open.Add(time.Minute)
	lc := NewLiveCandle(100, 5, open)
	lc.Apply(120, 1, open) // H=120

	completed, rolled := lc.Apply(130, 4, next)
	if !rolled || completed == nil {
		t.Fatal("crossing the boundary must roll and return the completed bar")
	}
	if completed.High != 120 || completed.Open != 100 {
		t.Fatalf("completed bar wrong: %+v", completed)
	}
	// Receiver is now the fresh bar seeded by the rolling tick.
	if lc.Open != 130 || lc.High != 130 || lc.Low != 130 || lc.Close != 130 || lc.Volume != 4 || !lc.OpenTime.Equal(next) {
		t.Fatalf("new bar not seeded correctly: %+v", lc)
	}
}

func TestTruncateToInterval(t *testing.T) {
	ts := time.Date(2026, 3, 17, 13, 47, 33, 0, time.UTC) // a Tuesday
	cases := map[string]time.Time{
		"1m": time.Date(2026, 3, 17, 13, 47, 0, 0, time.UTC),
		"1h": time.Date(2026, 3, 17, 13, 0, 0, 0, time.UTC),
		"4h": time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC),
		"1D": time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC),
		"1W": time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC), // Monday of that week
		"1M": time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	for interval, want := range cases {
		if got := TruncateToInterval(ts, interval); !got.Equal(want) {
			t.Errorf("TruncateToInterval(%s) = %v, want %v", interval, got, want)
		}
	}
}
