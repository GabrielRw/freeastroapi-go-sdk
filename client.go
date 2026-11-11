package freeastro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

const (
	// DefaultBaseURL is the production FreeAstroAPI server.
	DefaultBaseURL = "https://api.freeastroapi.com"
	// DefaultTimeout bounds each attempt of a request.
	DefaultTimeout = 20 * time.Second
	// DefaultMaxRetries is the number of retries after the first attempt.
	DefaultMaxRetries = 2

	// APIKeyEnv is the environment variable read when no API key is passed to NewClient.
	APIKeyEnv = "FREEASTRO_API_KEY"
)

// ErrMissingAPIKey is returned by NewClient when no API key is configured.
var ErrMissingAPIKey = errors.New("freeastro: no API key: pass WithAPIKey or set " + APIKeyEnv)

// HTTPDoer sends HTTP requests. *http.Client implements it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClient returns a client for FreeAstroAPI.
//
// The API key comes from WithAPIKey or, when that is not given, from the FREEASTRO_API_KEY
// environment variable. Keep API keys on servers and other trusted runtimes.
//
//	client, err := freeastro.NewClient(freeastro.WithAPIKey(os.Getenv("FREEASTRO_API_KEY")))
func NewClient(opts ...Option) (*Client, error) {
	s := settings{
		baseURL:    DefaultBaseURL,
		timeout:    DefaultTimeout,
		maxRetries: DefaultMaxRetries,
		baseDelay:  DefaultRetryBaseDelay,
		maxDelay:   DefaultRetryMaxDelay,
		headers:    http.Header{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	if strings.TrimSpace(s.apiKey) == "" {
		s.apiKey = os.Getenv(APIKeyEnv)
	}
	s.apiKey = strings.TrimSpace(s.apiKey)
	if s.apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	base, err := url.Parse(strings.TrimRight(s.baseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("freeastro: invalid base URL %q", s.baseURL)
	}
	if s.httpClient == nil {
		s.httpClient = &http.Client{}
	}
	if s.maxRetries < 0 {
		s.maxRetries = 0
	}

	userAgent := "freeastroapi-go/" + Version
	if s.userAgent != "" {
		userAgent = s.userAgent + " " + userAgent
	}
	c := &core{
		apiKey:     s.apiKey,
		baseURL:    base.String(),
		http:       s.httpClient,
		timeout:    s.timeout,
		maxRetries: s.maxRetries,
		baseDelay:  s.baseDelay,
		maxDelay:   s.maxDelay,
		headers:    s.headers,
		userAgent:  userAgent,
		sleep:      sleepContext,
		jitter:     fullJitter,
	}
	low, err := api.NewClientWithResponses(c.baseURL, api.WithHTTPClient(c))
	if err != nil {
		return nil, fmt.Errorf("freeastro: %w", err)
	}
	c.api = low

	client := &Client{core: c}
	client.initServices()
	return client, nil
}

// BaseURL returns the server URL the client sends requests to.
func (c *Client) BaseURL() string { return c.core.baseURL }

// Do sends a request to any API path and decodes the response into out. It is an escape
// hatch for endpoints that are newer than this SDK.
//
// path is relative to the base URL (for example "/api/v1/natal/calculate"). query may be
// nil. A non-nil body is sent as JSON. out may be nil (the body is discarded), a *[]byte or
// *Content (the raw body), or any value json.Unmarshal accepts.
//
// Do applies the same authentication, retries and error mapping as the typed methods.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any, opts ...RequestOption) error {
	method = strings.ToUpper(method)
	target := c.core.baseURL + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("freeastro: encode request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	accept := "application/json"
	switch out.(type) {
	case *[]byte, *Content:
		accept = "*/*"
	}
	// Known operations keep their retry policy (see SideEffectingOperations) through Do.
	opID := operationRoutes()[method+" /"+strings.TrimLeft(path, "/")]
	req, err := http.NewRequestWithContext(c.core.call(ctx, opID, accept, opts), method, target, reader)
	if err != nil {
		return fmt.Errorf("freeastro: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.core.Do(req)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return &NetworkError{Err: err}
	}

	switch dst := out.(type) {
	case nil:
		return nil
	case *[]byte:
		*dst = data
		return nil
	case *Content:
		*dst = *newContent(resp, data)
		return nil
	default:
		if len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return &DecodeError{StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: data, Err: err}
		}
		return nil
	}
}

// operationRoutes maps "METHOD /path" to the operation ID, for operations without path
// parameters.
var operationRoutes = sync.OnceValue(func() map[string]string {
	routes := make(map[string]string, len(operations))
	for _, op := range operations {
		routes[op.Method+" "+op.Path] = op.OperationID
	}
	return routes
})

// Ptr returns a pointer to v. Optional request fields are pointers:
//
//	api.V2GeoSearchParams{Q: "Paris", Limit: freeastro.Ptr(3)}
func Ptr[T any](v T) *T { return &v }
