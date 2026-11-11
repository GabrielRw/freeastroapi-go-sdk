package freeastro

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

// core holds the client configuration and implements api.HttpRequestDoer, so every request
// made by the generated low-level client goes through authentication, retries and error
// mapping here.
type core struct {
	apiKey     string
	baseURL    string
	http       HTTPDoer
	timeout    time.Duration
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	headers    http.Header
	userAgent  string
	sleep      func(ctx context.Context, d time.Duration) error
	jitter     func(max time.Duration) time.Duration

	api *api.ClientWithResponses
}

type callConfig struct {
	operationID    string
	accept         string
	idempotencyKey string
	retry          *bool
	maxRetries     *int
	timeout        *time.Duration
	headers        http.Header
	rawResponse    **http.Response
}

type callKey struct{}

// call attaches the per-call configuration to ctx; core.Do reads it back.
func (c *core) call(ctx context.Context, operationID, accept string, opts []RequestOption) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := &callConfig{operationID: operationID, accept: accept}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return context.WithValue(ctx, callKey{}, cfg)
}

// Do sends req with retries. A 2xx response is returned with its body fully read into memory;
// any other outcome is returned as an error (*APIError and friends, *NetworkError,
// *TimeoutError, or the context's error).
func (c *core) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	cfg, _ := ctx.Value(callKey{}).(*callConfig)
	if cfg == nil {
		cfg = &callConfig{accept: "application/json"}
	}

	header := req.Header
	for k, v := range c.headers {
		header[k] = append([]string(nil), v...)
	}
	if cfg.accept != "" && header.Get("Accept") == "" {
		header.Set("Accept", cfg.accept)
	}
	for k, v := range cfg.headers {
		header[k] = append([]string(nil), v...)
	}
	if cfg.idempotencyKey != "" {
		header.Set("Idempotency-Key", cfg.idempotencyKey)
	}
	header.Set("X-API-Key", c.apiKey)
	header.Set("User-Agent", c.userAgent)

	policy := c.policy(req, cfg)
	timeout := c.timeout
	if cfg.timeout != nil {
		timeout = *cfg.timeout
	}

	for attempt := 0; ; attempt++ {
		attemptReq := req
		if attempt > 0 {
			var err error
			if attemptReq, err = rewind(req); err != nil {
				return nil, &NetworkError{Err: err}
			}
		}
		canRetry := attempt < policy.maxRetries
		resp, body, err := c.send(attemptReq, timeout)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if policy.transient && canRetry {
				if err := c.sleep(ctx, c.backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
				return nil, &TimeoutError{NetworkError{Err: err}}
			}
			return nil, &NetworkError{Err: err}
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body = io.NopCloser(bytes.NewReader(body))
			if cfg.rawResponse != nil {
				*cfg.rawResponse = resp
			}
			return resp, nil
		}

		apiErr := errorFromResponse(resp, body)
		switch {
		case resp.StatusCode == http.StatusTooManyRequests && policy.rateLimit && canRetry:
			wait := apiErr.(*RateLimitError).RetryAfter
			if wait > MaxRetryAfter {
				return nil, apiErr // too long to wait: let the caller decide
			}
			if wait <= 0 {
				wait = c.backoff(attempt)
			}
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		case transientStatuses[resp.StatusCode] && policy.transient && canRetry:
			if err := c.sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, apiErr
	}
}

// send performs one attempt and reads the whole body, so the per-attempt timeout covers
// the body too.
func (c *core) send(req *http.Request, timeout time.Duration) (*http.Response, []byte, error) {
	ctx := req.Context()
	cancel := context.CancelFunc(func() {})
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}

func rewind(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return clone, nil
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
