package freeastro

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

type recorded struct {
	method string
	path   string
	query  string
	header http.Header
	body   []byte
}

type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
}

// newServer starts a test server whose handler answers request number i (0-based).
func newServer(t *testing.T, handler func(i int, w http.ResponseWriter, r *http.Request)) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fs.mu.Lock()
		i := len(fs.requests)
		fs.requests = append(fs.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
		fs.mu.Unlock()
		handler(i, w, r)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fakeServer) calls() []recorded {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]recorded(nil), fs.requests...)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// newTestClient returns a client for srv whose retry sleeps are recorded instead of waited.
func newTestClient(t *testing.T, srv *fakeServer, opts ...Option) (*Client, *[]time.Duration) {
	t.Helper()
	opts = append([]Option{WithAPIKey("test-key"), WithBaseURL(srv.URL)}, opts...)
	client, err := NewClient(opts...)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sleeps []time.Duration
	// No jitter: each backoff is its ceiling, so the recorded sleeps are deterministic.
	client.core.jitter = func(max time.Duration) time.Duration { return max }
	client.core.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		sleeps = append(sleeps, d)
		mu.Unlock()
		return ctx.Err()
	}
	return client, &sleeps
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	t.Setenv(APIKeyEnv, "")
	if _, err := NewClient(); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey", err)
	}
	if _, err := NewClient(WithAPIKey("   ")); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("blank key: err = %v, want ErrMissingAPIKey", err)
	}
	if _, err := NewClient(WithAPIKey("k"), WithBaseURL("not a url")); err == nil {
		t.Fatal("invalid base URL accepted")
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Setenv(APIKeyEnv, "")
	client, err := NewClient(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if client.BaseURL() != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", client.BaseURL())
	}
	if client.core.timeout != DefaultTimeout || client.core.maxRetries != DefaultMaxRetries {
		t.Fatalf("timeout %v retries %d", client.core.timeout, client.core.maxRetries)
	}
}

func TestAPIKeyFromEnvironment(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"count": 0, "results": []any{}})
	})
	t.Setenv(APIKeyEnv, "env-key")
	client, err := NewClient(WithBaseURL(srv.URL + "/"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "Paris"}); err != nil {
		t.Fatal(err)
	}
	if got := srv.calls()[0].header.Get("X-API-Key"); got != "env-key" {
		t.Fatalf("x-api-key = %q, want env-key", got)
	}
}

func TestQueryParamCallSendsAuthAndDecodes(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"count":   1,
			"results": []any{map[string]any{"name": "Paris", "country": "FR", "lat": 48.8566, "lng": 2.3522, "timezone": "Europe/Paris", "population": 2138551}},
		})
	})
	client, _ := newTestClient(t, srv, WithHeader("X-Trace", "abc"), WithUserAgent("my-app/1.0"))

	cities, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "Paris", Limit: Ptr(3), Country: Ptr("FR")})
	if err != nil {
		t.Fatal(err)
	}
	if cities.Count != 1 || cities.Results[0].Timezone != "Europe/Paris" || cities.Results[0].Lat != 48.8566 {
		t.Fatalf("decoded %+v", cities)
	}

	req := srv.calls()[0]
	if req.method != "GET" || req.path != "/api/v2/geo/search" {
		t.Fatalf("%s %s", req.method, req.path)
	}
	for _, want := range []string{"q=Paris", "limit=3", "country=FR"} {
		if !strings.Contains(req.query, want) {
			t.Errorf("query %q lacks %q", req.query, want)
		}
	}
	checks := map[string]string{
		"X-Api-Key":  "test-key",
		"User-Agent": "my-app/1.0 freeastroapi-go/" + Version,
		"Accept":     "application/json",
		"X-Trace":    "abc",
	}
	for k, want := range checks {
		if got := req.header.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
}

func TestBodyCall(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"point": "sun", "sign": "Gemini", "sign_id": "gemini", "degree": 24.1, "abs_pos": 84.1, "house": 10})
	})
	client, _ := newTestClient(t, srv)

	sun, err := client.Western.Signs.Sun(context.Background(), api.NatalChartRequest{
		Year: 1990, Month: 6, Day: 15, Hour: Ptr(14), Minute: Ptr(30), City: Ptr("Paris"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sun.Sign != "Gemini" || sun.House != 10 || sun.AbsPos != 84.1 {
		t.Fatalf("decoded %+v", sun)
	}

	req := srv.calls()[0]
	if req.method != "POST" || req.path != "/api/v1/western/signs/sun" {
		t.Fatalf("%s %s", req.method, req.path)
	}
	if ct := req.header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	var sent map[string]any
	if err := json.Unmarshal(req.body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["year"] != float64(1990) || sent["city"] != "Paris" || sent["minute"] != float64(30) {
		t.Fatalf("body %s", req.body)
	}
	if _, ok := sent["lat"]; ok {
		t.Fatalf("unset optional field sent: %s", req.body)
	}
}

func TestNatalCalculateIsTyped(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"planets": []any{map[string]any{"name": "Sun", "sign": "Gem"}}})
	})
	client, _ := newTestClient(t, srv)
	chart, err := client.Natal.Calculate(context.Background(), nil, api.NatalChartRequest{Year: 1990, Month: 6, Day: 15})
	if err != nil {
		t.Fatal(err)
	}
	if chart.Planets == nil || (*chart.Planets)[0].Name != "Sun" || (*chart.Planets)[0].Sign != "Gem" {
		t.Fatalf("chart %+v", chart)
	}
}

func TestRenamedResponseWrapperIsTyped(t *testing.T) {
	// The schema SimpleNatalResponse makes oapi-codegen rename the operation's wrapper to
	// SimpleNatalResponse2; the method must still return the typed schema.
	m, ok := reflect.TypeOf(&NatalService{}).MethodByName("Simple")
	if !ok || m.Type.Out(0) != reflect.TypeOf(&api.SimpleNatalResponse{}) {
		t.Fatalf("Natal.Simple returns %v", m.Type.Out(0))
	}
}

func TestPathParamCall(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{})
	})
	client, _ := newTestClient(t, srv)
	date := api.Date{Time: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)}
	if _, err := client.Chinese.Calendar(context.Background(), date, &api.V1ChineseCalendarDateParams{Lang: Ptr("en")}); err != nil {
		t.Fatal(err)
	}
	req := srv.calls()[0]
	if req.path != "/api/v1/chinese/calendar/2026-02-17" || req.query != "lang=en" {
		t.Fatalf("%s?%s", req.path, req.query)
	}
}

func TestBinaryResponses(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}
	srv := newServer(t, func(i int, w http.ResponseWriter, _ *http.Request) {
		if i == 0 {
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			_, _ = io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"/>`)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	client, _ := newTestClient(t, srv)
	ctx := context.Background()

	svg, err := client.Vedic.KPRender(ctx, api.KPChartRenderRequest{Year: 1990, Month: 6, Day: 15})
	if err != nil {
		t.Fatal(err)
	}
	if svg.MediaType() != "image/svg+xml" || !strings.HasPrefix(svg.String(), "<svg") || svg.IsJSON() {
		t.Fatalf("svg %q %q", svg.ContentType, svg.Data)
	}
	if accept := srv.calls()[0].header.Get("Accept"); accept != "image/png, image/svg+xml" {
		t.Fatalf("accept %q", accept)
	}

	img, err := client.Vedic.Visual.Chart(ctx, api.VedicVisualChartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if img.MediaType() != "image/png" || string(img.Data) != string(png) {
		t.Fatalf("png %q %v", img.ContentType, img.Data)
	}
}

func TestRetriesWithBackoff(t *testing.T) {
	srv := newServer(t, func(i int, w http.ResponseWriter, _ *http.Request) {
		if i < 2 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"detail": "busy"})
			return
		}
		writeJSON(w, 200, map[string]any{"count": 0, "results": []any{}})
	})
	client, sleeps := newTestClient(t, srv)
	if _, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "Paris"}); err != nil {
		t.Fatal(err)
	}
	if n := len(srv.calls()); n != 3 {
		t.Fatalf("%d attempts, want 3", n)
	}
	// Default policy: 2 retries, 0.5s doubling (jitter disabled in tests).
	if want := []time.Duration{500 * time.Millisecond, time.Second}; !equalDurations(*sleeps, want) {
		t.Fatalf("sleeps %v, want %v", *sleeps, want)
	}
}

func TestBackoffCapAndJitter(t *testing.T) {
	c := &core{baseDelay: DefaultRetryBaseDelay, maxDelay: DefaultRetryMaxDelay, jitter: func(max time.Duration) time.Duration { return max }}
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for attempt, w := range want {
		if got := c.backoff(attempt); got != w {
			t.Errorf("backoff(%d) = %v, want %v", attempt, got, w)
		}
	}
	if got := c.backoff(200); got != 8*time.Second {
		t.Errorf("backoff(200) = %v, want the cap", got)
	}
	for i := 0; i < 1000; i++ {
		if d := fullJitter(time.Second); d < 0 || d > time.Second {
			t.Fatalf("fullJitter(1s) = %v", d)
		}
	}
}

func TestRateLimitRetries(t *testing.T) {
	ctx := context.Background()
	sun := api.NatalChartRequest{Year: 2000, Month: 1, Day: 1}

	t.Run("POST honours Retry-After and retry_after_ms", func(t *testing.T) {
		srv := newServer(t, func(i int, w http.ResponseWriter, _ *http.Request) {
			switch i {
			case 0:
				w.Header().Set("Retry-After", "2")
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "detail": "slow down"})
			case 1:
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after_ms": 1500})
			default:
				writeJSON(w, 200, map[string]any{"sign": "Capricorn"})
			}
		})
		client, sleeps := newTestClient(t, srv)
		if _, err := client.Western.Signs.Sun(ctx, sun); err != nil {
			t.Fatal(err)
		}
		if want := []time.Duration{2 * time.Second, 1500 * time.Millisecond}; !equalDurations(*sleeps, want) {
			t.Fatalf("sleeps %v, want %v", *sleeps, want)
		}
	})
	t.Run("HTTP-date Retry-After and backoff without a hint", func(t *testing.T) {
		srv := newServer(t, func(i int, w http.ResponseWriter, _ *http.Request) {
			switch i {
			case 0:
				w.Header().Set("Retry-After", time.Now().Add(10*time.Second).UTC().Format(http.TimeFormat))
				writeJSON(w, http.StatusTooManyRequests, map[string]any{})
			case 1:
				writeJSON(w, http.StatusTooManyRequests, map[string]any{})
			default:
				writeJSON(w, 200, map[string]any{"count": 0, "results": []any{}})
			}
		})
		client, sleeps := newTestClient(t, srv)
		if _, err := client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "x"}); err != nil {
			t.Fatal(err)
		}
		got := *sleeps
		if len(got) != 2 || got[0] < 8*time.Second || got[0] > 10*time.Second || got[1] != time.Second {
			t.Fatalf("sleeps %v, want ~10s then the 1s backoff", got)
		}
	})
	t.Run("side-effecting operation without a key", func(t *testing.T) {
		srv := newServer(t, func(i int, w http.ResponseWriter, _ *http.Request) {
			if i == 0 {
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"retry_after_ms": 100})
				return
			}
			writeJSON(w, 200, map[string]any{})
		})
		client, _ := newTestClient(t, srv)
		if _, err := client.Vedic.Chat(ctx, nil, api.VedicChatRequest{}); err != nil {
			t.Fatal(err)
		}
		if n := len(srv.calls()); n != 2 {
			t.Fatalf("%d attempts, want 2 (429 is always retried)", n)
		}
	})
	t.Run("wait over 30s is not slept", func(t *testing.T) {
		srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "120")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "detail": "slow down"})
		})
		client, sleeps := newTestClient(t, srv)
		_, err := client.Western.Signs.Sun(ctx, sun)
		var rateLimited *RateLimitError
		if !errors.As(err, &rateLimited) || rateLimited.RetryAfter != 120*time.Second {
			t.Fatalf("err = %#v", err)
		}
		if rateLimited.Code != "rate_limited" || rateLimited.Message != "slow down" {
			t.Fatalf("code %q message %q", rateLimited.Code, rateLimited.Message)
		}
		if n := len(srv.calls()); n != 1 || len(*sleeps) != 0 {
			t.Fatalf("%d attempts, sleeps %v; want 1 attempt and no sleep", n, *sleeps)
		}
	})
	t.Run("retries exhausted", func(t *testing.T) {
		srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"retry_after_ms": 50})
		})
		client, _ := newTestClient(t, srv)
		_, err := client.Western.Signs.Sun(ctx, sun)
		var rateLimited *RateLimitError
		if !errors.As(err, &rateLimited) || rateLimited.RetryAfter != 50*time.Millisecond || len(srv.calls()) != 3 {
			t.Fatalf("err %#v after %d attempts", err, len(srv.calls()))
		}
	})
}

func TestRetriesExhausted(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "req-9")
		writeJSON(w, http.StatusBadGateway, map[string]any{"detail": "upstream down"})
	})
	client, _ := newTestClient(t, srv, WithMaxRetries(3))
	_, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "x"})
	var serverErr *ServerError
	if !errors.As(err, &serverErr) || serverErr.StatusCode != 502 || serverErr.RequestID != "req-9" {
		t.Fatalf("err = %#v", err)
	}
	if n := len(srv.calls()); n != 4 {
		t.Fatalf("%d attempts, want 4", n)
	}
}

func TestTransientRetryPolicy(t *testing.T) {
	ctx := context.Background()
	sun := api.NatalChartRequest{Year: 2000, Month: 1, Day: 1}
	failTwice := func(i int, w http.ResponseWriter, _ *http.Request) {
		if i%3 < 2 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{})
			return
		}
		writeJSON(w, 200, map[string]any{"sign": "Capricorn"})
	}
	vedicChat := func(client *Client, opts ...RequestOption) error {
		_, err := client.Vedic.Chat(ctx, nil, api.VedicChatRequest{}, opts...)
		return err
	}

	cases := []struct {
		name     string
		call     func(*Client) error
		attempts int
	}{
		{"calculation POST is retried", func(c *Client) error { _, err := c.Western.Signs.Sun(ctx, sun); return err }, 3},
		{"GET is retried", func(c *Client) error { _, err := c.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "x"}); return err }, 3},
		{"side-effecting operation is not retried", func(c *Client) error { return vedicChat(c) }, 1},
		{"side-effecting operation with a key", func(c *Client) error { return vedicChat(c, WithIdempotencyKey("key-1")) }, 3},
		{"side-effecting operation opted in", func(c *Client) error { return vedicChat(c, WithRetry(true)) }, 3},
		{"natal calculate is side-effecting", func(c *Client) error {
			_, err := c.Natal.Calculate(ctx, nil, api.NatalChartRequest{Year: 2000, Month: 1, Day: 1})
			return err
		}, 1},
		{"opt-out", func(c *Client) error { _, err := c.Western.Signs.Sun(ctx, sun, WithRetry(false)); return err }, 1},
		{"per-request max retries", func(c *Client) error {
			_, err := c.Western.Signs.Sun(ctx, sun, WithRequestMaxRetries(1))
			return err
		}, 2},
		{"Do keeps the policy of a known operation", func(c *Client) error {
			return c.Do(ctx, "POST", "/api/v2/vedic/chat", nil, map[string]any{}, nil)
		}, 1},
		{"Do retries other paths", func(c *Client) error {
			return c.Do(ctx, "POST", "/api/v1/example", nil, map[string]any{}, nil)
		}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, failTwice)
			client, _ := newTestClient(t, srv)
			err := tc.call(client)
			if got := len(srv.calls()); got != tc.attempts {
				t.Fatalf("%d attempts, want %d (err %v)", got, tc.attempts, err)
			}
			if tc.attempts == 3 && err != nil {
				t.Fatal(err)
			}
			var serverErr *ServerError
			if tc.attempts < 3 && !errors.As(err, &serverErr) {
				t.Fatalf("err = %#v, want *ServerError", err)
			}
		})
	}

	t.Run("retried requests keep key and body", func(t *testing.T) {
		srv := newServer(t, failTwice)
		client, _ := newTestClient(t, srv)
		if _, err := client.Western.Signs.Sun(ctx, sun, WithIdempotencyKey("key-1")); err != nil {
			t.Fatal(err)
		}
		for _, c := range srv.calls() {
			if c.header.Get("Idempotency-Key") != "key-1" || !strings.Contains(string(c.body), `"year":2000`) {
				t.Fatalf("retried request lost its key or body: %v %s", c.header, c.body)
			}
		}
	})
	t.Run("other 4xx are never retried", func(t *testing.T) {
		for _, status := range []int{400, 401, 403, 404, 409, 422} {
			srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, status, map[string]any{"detail": "no"})
			})
			client, _ := newTestClient(t, srv)
			if _, err := client.Western.Signs.Sun(ctx, sun, WithRetry(true)); err == nil {
				t.Fatalf("%d: expected an error", status)
			}
			if n := len(srv.calls()); n != 1 {
				t.Fatalf("%d: %d attempts, want 1", status, n)
			}
		}
	})
	t.Run("side-effecting operations exist", func(t *testing.T) {
		known := map[string]bool{}
		for _, op := range Operations() {
			known[op.OperationID] = true
		}
		ids := SideEffectingOperations()
		if len(ids) != 7 {
			t.Fatalf("SideEffectingOperations() = %v", ids)
		}
		for _, id := range ids {
			if !known[id] || !IsSideEffecting(id) {
				t.Errorf("%s is not an operation of the spec", id)
			}
		}
	})
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{401, `{"detail":"Invalid API key"}`, func(err error) bool { var e *AuthenticationError; return errors.As(err, &e) }},
		{403, `{"detail":"Plan required"}`, func(err error) bool { var e *PermissionError; return errors.As(err, &e) }},
		{404, `{"detail":"Not Found"}`, func(err error) bool { var e *NotFoundError; return errors.As(err, &e) }},
		{409, `{"detail":"Conflict"}`, func(err error) bool { var e *ConflictError; return errors.As(err, &e) }},
		{400, `{"detail":"Bad"}`, func(err error) bool { var e *ValidationError; return errors.As(err, &e) }},
		{422, `{"detail":[{"loc":["body","year"],"msg":"field required","type":"missing"}]}`, func(err error) bool { var e *ValidationError; return errors.As(err, &e) }},
		{429, `{"detail":"Too many"}`, func(err error) bool {
			var e *RateLimitError
			return errors.As(err, &e) && e.RetryAfter == 30*time.Second
		}},
		{500, `{"detail":"boom"}`, func(err error) bool { var e *ServerError; return errors.As(err, &e) }},
		{418, `teapot`, func(err error) bool { var e *APIError; return errors.As(err, &e) && e.Message == "I'm a teapot" }},
	}
	for _, tc := range cases {
		srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		})
		client, _ := newTestClient(t, srv, WithMaxRetries(0))
		_, err := client.Western.Signs.Sun(context.Background(), api.NatalChartRequest{})
		if err == nil || !tc.check(err) {
			t.Errorf("HTTP %d: unexpected error %#v", tc.status, err)
			continue
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status || string(apiErr.Body) != tc.body {
			t.Errorf("HTTP %d: APIError %#v", tc.status, apiErr)
		}
	}
}

func TestErrorDetails(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 401, map[string]any{"message": "Invalid API key", "code": "invalid_api_key", "trace_id": "trace-1"})
	})
	client, _ := newTestClient(t, srv)
	_, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "x"})
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v", err)
	}
	if authErr.Message != "Invalid API key" || authErr.Code != "invalid_api_key" || authErr.RequestID != "trace-1" {
		t.Fatalf("%+v", authErr.APIError)
	}
	want := "freeastro: HTTP 401: Invalid API key (code invalid_api_key) [request trace-1]"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
	var details struct{ Code string }
	if err := authErr.Details(&details); err != nil || details.Code != "invalid_api_key" {
		t.Fatalf("details %+v %v", details, err)
	}
}

func TestErrorResponseShapes(t *testing.T) {
	cases := []struct {
		body          string
		message, code string
	}{
		{`{"error":"quota_exceeded","detail":"Monthly quota reached","docs":"https://x"}`, "Monthly quota reached", "quota_exceeded"},
		{`{"detail":{"error":"invalid_city","message":"City not found","city":"Atlantis"}}`, "City not found", "invalid_city"},
		{`{"error":"Something broke"}`, "Something broke", ""},
	}
	for _, tc := range cases {
		srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, tc.body)
		})
		client, _ := newTestClient(t, srv)
		_, err := client.Western.Signs.Sun(context.Background(), api.NatalChartRequest{})
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Message != tc.message || apiErr.Code != tc.code {
			t.Errorf("%s: %#v", tc.body, apiErr)
			continue
		}
		if body := apiErr.ErrorResponse(); body == nil || body.Detail == nil && body.Error == nil {
			t.Errorf("%s: ErrorResponse() = %+v", tc.body, body)
		}
	}
}

func TestNetworkErrorAndTimeout(t *testing.T) {
	t.Run("network", func(t *testing.T) {
		srv := newServer(t, func(int, http.ResponseWriter, *http.Request) {})
		url := srv.URL
		srv.Close()
		client, err := NewClient(WithAPIKey("k"), WithBaseURL(url), WithMaxRetries(1), WithRetryDelay(time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "x"})
		var netErr *NetworkError
		var timeoutErr *TimeoutError
		if !errors.As(err, &netErr) || errors.As(err, &timeoutErr) {
			t.Fatalf("err = %#v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		srv := newServer(t, func(_ int, w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		defer close(release)
		client, _ := newTestClient(t, srv, WithTimeout(50*time.Millisecond), WithMaxRetries(1))
		_, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "x"})
		var timeoutErr *TimeoutError
		var netErr *NetworkError
		if !errors.As(err, &timeoutErr) || !errors.As(err, &netErr) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %#v", err)
		}
		if n := len(srv.calls()); n != 2 {
			t.Fatalf("%d attempts, want 2 (timeouts are retried)", n)
		}
	})
	t.Run("side-effecting operation is not retried after a network error", func(t *testing.T) {
		srv := newServer(t, func(int, http.ResponseWriter, *http.Request) {})
		url := srv.URL
		srv.Close()
		client, err := NewClient(WithAPIKey("k"), WithBaseURL(url), WithRetryDelay(time.Millisecond, time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		var attempts int
		client.core.sleep = func(ctx context.Context, d time.Duration) error { attempts++; return nil }
		_, err = client.AI.Generate(context.Background(), api.AtomicReportRequest{})
		var netErr *NetworkError
		if !errors.As(err, &netErr) || attempts != 0 {
			t.Fatalf("err %#v after %d retries", err, attempts)
		}
		_, _ = client.AI.Generate(context.Background(), api.AtomicReportRequest{}, WithIdempotencyKey("k1"))
		if attempts != 2 {
			t.Fatalf("%d retries with an idempotency key, want 2", attempts)
		}
	})
	t.Run("caller context", func(t *testing.T) {
		srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 503, map[string]any{})
		})
		client, err := NewClient(WithAPIKey("k"), WithBaseURL(srv.URL))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err = client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "x"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %#v", err)
		}
		var timeoutErr *TimeoutError
		if errors.As(err, &timeoutErr) {
			t.Fatal("caller's deadline reported as TimeoutError")
		}
	})
}

func TestRawResponseAndRequestHeaders(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "41")
		writeJSON(w, 200, map[string]any{"count": 0, "results": []any{}})
	})
	client, _ := newTestClient(t, srv, WithHeader("X-Env", "client"))
	var resp *http.Response
	_, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{Q: "x"},
		WithRawResponse(&resp), WithRequestHeader("X-Env", "request"), WithRequestTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Header.Get("X-RateLimit-Remaining") != "41" {
		t.Fatalf("raw response %v", resp)
	}
	if got := srv.calls()[0].header.Values("X-Env"); len(got) != 1 || got[0] != "request" {
		t.Fatalf("X-Env %v", got)
	}
}

func TestDo(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "method": r.Method})
	})
	client, _ := newTestClient(t, srv)
	var out struct {
		OK     bool   `json:"ok"`
		Method string `json:"method"`
	}
	err := client.Do(context.Background(), "post", "/api/v1/example", map[string][]string{"lang": {"fr"}}, map[string]any{"a": 1}, &out)
	if err != nil || !out.OK || out.Method != "POST" {
		t.Fatalf("out %+v err %v", out, err)
	}
	req := srv.calls()[0]
	if req.path != "/api/v1/example" || req.query != "lang=fr" || string(req.body) != `{"a":1}` || req.header.Get("X-Api-Key") != "test-key" {
		t.Fatalf("%+v", req)
	}

	var raw []byte
	if err := client.Do(context.Background(), "GET", "api/v1/example", nil, nil, &raw); err != nil || !json.Valid(raw) {
		t.Fatalf("raw %s err %v", raw, err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"3":                             3 * time.Second,
		"1.5":                           1500 * time.Millisecond,
		"-4":                            0,
		"Thu, 01 Jan 2026 12:00:10 GMT": 10 * time.Second,
		"Thu, 01 Jan 2026 11:00:00 GMT": 0,
		"soon":                          0,
	}
	for in, want := range cases {
		if got, _ := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
