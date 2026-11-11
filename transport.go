package freeastro

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
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

	api *api.ClientWithResponses
}

type callConfig struct {
	operationID    string
	accept         string
	idempotencyKey string
	retry          *bool
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

var retryStatuses = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
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

	retryable := c.retryable(req, cfg)
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
		resp, body, err := c.send(attemptReq, timeout)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if retryable && attempt < c.maxRetries {
				if err := c.sleep(ctx, c.backoff(attempt, 0)); err != nil {
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

		if retryable && retryStatuses[resp.StatusCode] && attempt < c.maxRetries {
			wait, _ := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			if err := c.sleep(ctx, c.backoff(attempt, wait)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, errorFromResponse(resp, body)
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

func (c *core) retryable(req *http.Request, cfg *callConfig) bool {
	if c.maxRetries <= 0 {
		return false
	}
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false // the body cannot be replayed
	}
	if cfg.retry != nil {
		return *cfg.retry
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return req.Header.Get("Idempotency-Key") != ""
}

// backoff returns the delay before retry number attempt+1: Retry-After when the server sent
// one, otherwise base * 2^attempt, both capped at maxDelay.
func (c *core) backoff(attempt int, retryAfter time.Duration) time.Duration {
	delay := retryAfter
	if delay <= 0 {
		delay = time.Duration(float64(c.baseDelay) * math.Pow(2, float64(attempt)))
	}
	if c.maxDelay > 0 && delay > c.maxDelay {
		delay = c.maxDelay
	}
	return delay
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
