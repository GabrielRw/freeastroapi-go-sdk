package freeastro

import (
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"time"
)

// Retry policy defaults (shared by every FreeAstroAPI SDK).
const (
	// DefaultRetryBaseDelay is the backoff before the first retry; it doubles on each retry.
	DefaultRetryBaseDelay = 500 * time.Millisecond
	// DefaultRetryMaxDelay caps the computed backoff.
	DefaultRetryMaxDelay = 8 * time.Second
	// MaxRetryAfter is the longest server-requested wait (Retry-After or retry_after_ms) the
	// client sleeps through. A longer wait is not slept: the *RateLimitError is returned at
	// once with RetryAfter set.
	MaxRetryAfter = 30 * time.Second
)

// sideEffectingOperations may have effects beyond computing a result (creating a paid report
// job, spending AI credits, ...). They are retried after 408, 5xx, network errors and
// timeouts only when the call carries an idempotency key (WithIdempotencyKey) or opts in
// with WithRetry(true). A 429 is retried for every operation, since the request was
// rejected before it ran.
var sideEffectingOperations = map[string]bool{
	"v1_natal_calculate":           true, // may create a paid report job
	"v1_ai_generate":               true,
	"v2_vedic_chat":                true,
	"v2_western_chat":              true,
	"v2_vedic_qa":                  true,
	"v1_western_transits_timeline": true,
	"v1_western_transits_insights": true,
}

// SideEffectingOperations returns, sorted, the operation IDs whose transient failures
// (408, 5xx, network errors, timeouts) are retried only with an idempotency key or
// WithRetry(true).
func SideEffectingOperations() []string {
	ids := make([]string, 0, len(sideEffectingOperations))
	for id := range sideEffectingOperations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IsSideEffecting reports whether operationID is one of SideEffectingOperations.
func IsSideEffecting(operationID string) bool { return sideEffectingOperations[operationID] }

// transientStatuses are retried like network errors (subject to the side-effect rule).
var transientStatuses = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// retryPolicy is the resolved policy for one call.
type retryPolicy struct {
	maxRetries int
	transient  bool // retry 408, 5xx, network errors and timeouts
	rateLimit  bool // retry 429
}

func (c *core) policy(req *http.Request, cfg *callConfig) retryPolicy {
	p := retryPolicy{maxRetries: c.maxRetries}
	if cfg.maxRetries != nil {
		p.maxRetries = *cfg.maxRetries
	}
	if p.maxRetries <= 0 {
		return retryPolicy{}
	}
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return retryPolicy{} // the body cannot be replayed
	}
	if cfg.retry != nil {
		if !*cfg.retry {
			return retryPolicy{}
		}
		p.transient, p.rateLimit = true, true
		return p
	}
	p.rateLimit = true
	p.transient = !sideEffectingOperations[cfg.operationID] || req.Header.Get("Idempotency-Key") != ""
	return p
}

// backoff returns the delay before retry number attempt+1: full jitter over
// base * 2^attempt, capped at maxDelay.
func (c *core) backoff(attempt int) time.Duration {
	ceiling := time.Duration(float64(c.baseDelay) * math.Pow(2, float64(attempt)))
	if c.maxDelay > 0 && (ceiling > c.maxDelay || ceiling <= 0) {
		ceiling = c.maxDelay
	}
	if ceiling <= 0 {
		return 0
	}
	return c.jitter(ceiling)
}

// fullJitter returns a uniformly random duration in [0, max].
func fullJitter(max time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(max) + 1))
}
