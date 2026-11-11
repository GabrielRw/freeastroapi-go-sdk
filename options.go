package freeastro

import (
	"net/http"
	"time"
)

// Option configures a Client. Pass options to NewClient.
type Option func(*settings)

type settings struct {
	apiKey     string
	baseURL    string
	httpClient HTTPDoer
	timeout    time.Duration
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	headers    http.Header
	userAgent  string
}

// WithAPIKey sets the API key sent in the x-api-key header. When it is not given (or is
// empty), NewClient reads the FREEASTRO_API_KEY environment variable.
func WithAPIKey(key string) Option {
	return func(s *settings) { s.apiKey = key }
}

// WithBaseURL overrides the server URL (default https://api.freeastroapi.com).
func WithBaseURL(baseURL string) Option {
	return func(s *settings) { s.baseURL = baseURL }
}

// WithHTTPClient sets the HTTP client used to send requests (default: a new http.Client).
// Use it to configure proxies, transports or instrumentation.
func WithHTTPClient(client HTTPDoer) Option {
	return func(s *settings) { s.httpClient = client }
}

// WithTimeout bounds each attempt of a request (default 20s). Zero disables the per-attempt
// timeout; the context passed to each call still applies.
func WithTimeout(timeout time.Duration) Option {
	return func(s *settings) { s.timeout = timeout }
}

// WithMaxRetries sets how many times a retryable request is retried after the first attempt
// (default 2). Zero disables retries.
func WithMaxRetries(n int) Option {
	return func(s *settings) { s.maxRetries = n }
}

// WithRetryDelay sets the exponential backoff: the n-th retry waits base * 2^n, capped at
// max (defaults 250ms and 5s). A Retry-After header replaces the computed delay, still
// capped at max.
func WithRetryDelay(base, max time.Duration) Option {
	return func(s *settings) {
		s.baseDelay = base
		s.maxDelay = max
	}
}

// WithHeader adds a header to every request.
func WithHeader(key, value string) Option {
	return func(s *settings) { s.headers.Add(key, value) }
}

// WithUserAgent prepends a product token such as "my-app/1.2" to the User-Agent header
// (which is "freeastroapi-go/<version>" by default).
func WithUserAgent(product string) Option {
	return func(s *settings) { s.userAgent = product }
}

// RequestOption configures a single API call.
type RequestOption func(*callConfig)

// WithIdempotencyKey sends an Idempotency-Key header. It also makes a POST request
// eligible for automatic retries.
func WithIdempotencyKey(key string) RequestOption {
	return func(c *callConfig) { c.idempotencyKey = key }
}

// WithRetry forces automatic retries on (true) or off (false) for this call. By default
// GET, HEAD and OPTIONS requests, and requests with an idempotency key, are retried; other
// requests are not, because the SDK cannot know that replaying them is safe.
func WithRetry(enabled bool) RequestOption {
	return func(c *callConfig) { c.retry = &enabled }
}

// WithRequestTimeout overrides the client's per-attempt timeout for this call.
func WithRequestTimeout(timeout time.Duration) RequestOption {
	return func(c *callConfig) { c.timeout = &timeout }
}

// WithRequestHeader sets a header on this call, replacing any client-level value.
func WithRequestHeader(key, value string) RequestOption {
	return func(c *callConfig) {
		if c.headers == nil {
			c.headers = http.Header{}
		}
		c.headers.Set(key, value)
	}
}

// WithRawResponse stores the final HTTP response of a successful call in *dst, for example
// to read rate-limit headers or X-Request-Id. The body has already been read.
func WithRawResponse(dst **http.Response) RequestOption {
	return func(c *callConfig) { c.rawResponse = dst }
}
