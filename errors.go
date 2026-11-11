package freeastro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

// APIError is an error response from FreeAstroAPI. Every HTTP error is reported as one of
// the types below, all of which wrap an *APIError, so either form works:
//
//	var rateLimited *freeastro.RateLimitError
//	if errors.As(err, &rateLimited) {
//		time.Sleep(rateLimited.RetryAfter)
//	}
//
//	var apiErr *freeastro.APIError
//	if errors.As(err, &apiErr) {
//		log.Println(apiErr.StatusCode, apiErr.Code, apiErr.RequestID)
//	}
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Message is the error message from the response body, or the HTTP status text.
	Message string
	// Code is the machine-readable error code from the body, when present.
	Code string
	// RequestID identifies the request for support (X-Request-Id header or trace_id).
	RequestID string
	// Body is the raw response body.
	Body []byte
	// Header holds the response headers.
	Header http.Header
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "freeastro: HTTP %d", e.StatusCode)
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if e.Code != "" {
		b.WriteString(" (code " + e.Code + ")")
	}
	if e.RequestID != "" {
		b.WriteString(" [request " + e.RequestID + "]")
	}
	return b.String()
}

// Details decodes the JSON error body into v.
func (e *APIError) Details(v any) error { return json.Unmarshal(e.Body, v) }

// ErrorResponse decodes the body as the documented error shape (api.ErrorResponse). It
// returns nil when the body is not a JSON object. For a 422, the validation details of the
// api.HTTPValidationError shape are in Detail (use Details to decode them directly).
func (e *APIError) ErrorResponse() *api.ErrorResponse {
	var out api.ErrorResponse
	if err := json.Unmarshal(e.Body, &out); err != nil {
		return nil
	}
	return &out
}

// AuthenticationError is returned for HTTP 401: the API key is missing or invalid.
type AuthenticationError struct{ APIError }

// PermissionError is returned for HTTP 403: the key may not use this operation or plan.
type PermissionError struct{ APIError }

// NotFoundError is returned for HTTP 404.
type NotFoundError struct{ APIError }

// ConflictError is returned for HTTP 409.
type ConflictError struct{ APIError }

// ValidationError is returned for HTTP 400 and 422: the request was rejected as invalid.
// Body holds the validation details.
type ValidationError struct{ APIError }

// RateLimitError is returned for HTTP 429, after any automatic retries. It is returned at
// once, without retrying, when the server asks for a wait longer than MaxRetryAfter.
type RateLimitError struct {
	APIError
	// RetryAfter is how long the server asked the client to wait (the Retry-After header,
	// or retry_after_ms in the body), or zero.
	RetryAfter time.Duration
}

// ServerError is returned for HTTP 5xx responses.
type ServerError struct{ APIError }

func (e *AuthenticationError) Unwrap() error { return &e.APIError }
func (e *PermissionError) Unwrap() error     { return &e.APIError }
func (e *NotFoundError) Unwrap() error       { return &e.APIError }
func (e *ConflictError) Unwrap() error       { return &e.APIError }
func (e *ValidationError) Unwrap() error     { return &e.APIError }
func (e *RateLimitError) Unwrap() error      { return &e.APIError }
func (e *ServerError) Unwrap() error         { return &e.APIError }

// NetworkError reports a failure to reach the API or read its response.
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string {
	return "freeastro: unable to reach FreeAstroAPI: " + e.Err.Error()
}
func (e *NetworkError) Unwrap() error { return e.Err }

// TimeoutError reports that an attempt exceeded the client's timeout (WithTimeout or
// WithRequestTimeout). It wraps a *NetworkError. When the caller's own context expires,
// the context error is returned instead.
type TimeoutError struct{ NetworkError }

func (e *TimeoutError) Error() string { return "freeastro: request timed out: " + e.Err.Error() }
func (e *TimeoutError) Unwrap() error { return &e.NetworkError }

// DecodeError reports a successful response whose body could not be decoded.
type DecodeError struct {
	StatusCode  int
	ContentType string
	Body        []byte
	Err         error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("freeastro: cannot decode HTTP %d response (%s): %v", e.StatusCode, e.ContentType, e.Err)
}
func (e *DecodeError) Unwrap() error { return e.Err }

// errorFromResponse maps a non-2xx response to a typed error.
func errorFromResponse(resp *http.Response, body []byte) error {
	base := APIError{
		StatusCode: resp.StatusCode,
		Body:       body,
		Header:     resp.Header,
		RequestID:  resp.Header.Get("X-Request-Id"),
	}

	// Error bodies follow api.ErrorResponse: route errors carry "detail" (a message, or an
	// object with an "error" code and a "message"); gateway errors carry a top-level "error"
	// code and usually a "detail" message. 422 can also use api.HTTPValidationError.
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	detail, _ := payload["detail"].(map[string]any)
	base.Message = firstString(payload, "detail", "message", "error", "title")
	if base.Message == "" && detail != nil {
		base.Message = firstString(detail, "message", "detail", "msg")
	}
	if base.Message == "" {
		base.Message = strings.TrimSpace(strings.TrimPrefix(resp.Status, strconv.Itoa(resp.StatusCode)))
	}
	if base.Message == "" {
		base.Message = http.StatusText(resp.StatusCode)
	}
	base.Code = firstString(payload, "code", "error_code")
	if base.Code == "" && detail != nil {
		base.Code = firstString(detail, "error", "code", "error_code")
	}
	if code, ok := payload["error"].(string); base.Code == "" && ok && strings.TrimSpace(code) != base.Message {
		base.Code = strings.TrimSpace(code)
	}
	if base.RequestID == "" {
		base.RequestID = firstString(payload, "request_id", "trace_id")
	}

	switch code := resp.StatusCode; {
	case code == http.StatusUnauthorized:
		return &AuthenticationError{base}
	case code == http.StatusForbidden:
		return &PermissionError{base}
	case code == http.StatusNotFound:
		return &NotFoundError{base}
	case code == http.StatusConflict:
		return &ConflictError{base}
	case code == http.StatusBadRequest || code == http.StatusUnprocessableEntity:
		return &ValidationError{base}
	case code == http.StatusTooManyRequests:
		wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		if !ok {
			wait = retryAfterMillis(payload, detail)
		}
		return &RateLimitError{APIError: base, RetryAfter: wait}
	case code >= 500:
		return &ServerError{base}
	default:
		return &base
	}
}

// firstString returns the first non-empty string value among keys, also looking inside a
// nested "error" object ({"error": {"message": ..., "code": ...}}).
func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		return firstString(nested, keys...)
	}
	return ""
}

// retryAfterMillis reads retry_after_ms from the error body (top level or inside detail).
func retryAfterMillis(objects ...map[string]any) time.Duration {
	for _, obj := range objects {
		if ms, ok := obj["retry_after_ms"].(float64); ok && ms > 0 {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}
	return 0
}

// parseRetryAfter parses a Retry-After header in seconds or as an HTTP date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return time.Duration(seconds * float64(time.Second)), true
	}
	if at, err := http.ParseTime(value); err == nil {
		if d := at.Sub(now); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}
