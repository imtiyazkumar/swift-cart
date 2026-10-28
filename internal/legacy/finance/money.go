package finance

import (
	"errors"
	"math"
	"time"
)

// Commission rounds half-up once in minor units. Bound intermediates before multiplication.
func Commission(gross int64, bps int) (int64, error) {
	if gross < 0 || gross > math.MaxInt64/10000-1 || bps < 0 || bps > 10000 {
		return 0, errors.New("invalid commission basis")
	}
	return (gross*int64(bps) + 5000) / 10000, nil
}

func Cutoff(now, anchor time.Time, days int) time.Time {
	if days < 1 {
		days = 15
	}
	anchor = anchor.UTC()
	now = now.UTC()
	interval := time.Duration(days) * 24 * time.Hour
	if now.Before(anchor) {
		return anchor.Add(-interval)
	}
	return anchor.Add(time.Duration(now.Sub(anchor)/interval) * interval)
}
