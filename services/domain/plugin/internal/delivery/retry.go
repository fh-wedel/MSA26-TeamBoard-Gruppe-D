package delivery

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

var backoffSchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

const maxAttempts = 8

// ComputeNextAttempt returns the next scheduled time and true, or zero/false if max attempts reached.
func ComputeNextAttempt(attemptCount int) (time.Time, bool) {
	if attemptCount >= maxAttempts {
		return time.Time{}, false
	}
	idx := attemptCount - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(backoffSchedule) {
		idx = len(backoffSchedule) - 1
	}
	base := backoffSchedule[idx]
	// ±20% jitter
	jitter := time.Duration(rand.Int63n(int64(base)/5)) - base/10
	return time.Now().Add(base + jitter), true
}

// IsPermanentFailure returns true for HTTP status codes that should not be retried.
func IsPermanentFailure(statusCode int) bool {
	if statusCode == http.StatusGone {
		return true
	}
	// 4xx except 408, 425, 429 → permanent
	if statusCode >= 400 && statusCode < 500 {
		switch statusCode {
		case http.StatusRequestTimeout, 425, http.StatusTooManyRequests:
			return false
		}
		return true
	}
	return false
}

// ParseRetryAfter parses a Retry-After header value into a duration.
// Returns 0 if not parseable.
func ParseRetryAfter(header http.Header) time.Duration {
	val := header.Get("Retry-After")
	if val == "" {
		return 0
	}
	// Seconds format
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	// HTTP-date format
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}
