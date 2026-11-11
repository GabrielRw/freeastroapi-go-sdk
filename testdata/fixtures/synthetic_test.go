package freeastro

// Tests for an SDK generated from tests/fixtures/synthetic-spec.json (shared with the other
// FreeAstroAPI SDKs). Run by scripts/test-fixture.sh, which copies this file into a temporary
// module next to the generated code.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := NewClient(WithAPIKey("k"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSyntheticTypedResponses(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/things/create":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"t1","name":"one","owner":{"thing_id":"t0"}}`)
		case "/api/v2/things/create":
			if r.Header.Get("Accept") != "application/vnd.freeastro+json" {
				t.Errorf("accept %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "application/vnd.freeastro+json")
			_, _ = io.WriteString(w, `{"id":"t2","name":"two","owner":null}`)
		case "/api/v1/things/item/t9":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"thing_id":"t9"}`)
		}
	})
	ctx := context.Background()

	v1, err := client.Things.CreateV1(ctx, api.ThingInput{Name: "one"})
	if err != nil || v1.ID != "t1" || v1.Owner == nil || v1.Owner.ThingID != "t0" {
		t.Fatalf("CreateV1 %+v %v", v1, err)
	}
	v2, err := client.Things.Create(ctx, api.ThingInput{Name: "two"})
	if err != nil || v2.ID != "t2" || v2.Owner != nil {
		t.Fatalf("Create %+v %v", v2, err)
	}
	rec, err := client.Things.Item(ctx, "t9", &api.V1ThingsItemParams{})
	if err != nil || rec.ThingID != "t9" {
		t.Fatalf("Item %+v %v", rec, err)
	}
}

func TestSyntheticErrorResponses(t *testing.T) {
	cases := map[string]struct {
		status int
		ctype  string
		check  func(error) bool
	}{
		"/api/v1/things/create": {400, "application/problem+json", func(err error) bool { var e *ValidationError; return errors.As(err, &e) }},
		"/api/v1/things/item/x": {404, "application/json", func(err error) bool { var e *NotFoundError; return errors.As(err, &e) }},
		"/api/v1/docs/pdf":      {409, "application/json", func(err error) bool { var e *ConflictError; return errors.As(err, &e) }},
	}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		c := cases[r.URL.Path]
		w.Header().Set("Content-Type", c.ctype)
		w.WriteHeader(c.status)
		_, _ = io.WriteString(w, `{"detail":"nope","code":"E_TEST","trace_id":"tr-1"}`)
	})
	ctx := context.Background()
	errs := map[string]error{}
	_, errs["/api/v1/things/create"] = client.Things.CreateV1(ctx, api.ThingInput{Name: "x"})
	_, errs["/api/v1/things/item/x"] = client.Things.Item(ctx, "x", nil)
	_, errs["/api/v1/docs/pdf"] = client.Docs.PDF(ctx, &api.V1DocsPdfParams{JobID: "j"})
	for path, err := range errs {
		var apiErr *APIError
		if !cases[path].check(err) || !errors.As(err, &apiErr) {
			t.Errorf("%s: %#v", path, err)
			continue
		}
		if apiErr.Message != "nope" || apiErr.Code != "E_TEST" || apiErr.RequestID != "tr-1" {
			t.Errorf("%s: %+v", path, apiErr)
		}
		var body api.ErrorResponse
		if err := apiErr.Details(&body); err != nil || body.Detail != "nope" || body.TraceID == nil || *body.TraceID != "tr-1" {
			t.Errorf("%s: ErrorResponse %+v %v", path, body, err)
		}
	}
}

func TestSyntheticContentResponses(t *testing.T) {
	bodies := map[string][2]string{
		"/api/v1/render/svg":    {"image/svg+xml", "<svg/>"},
		"/api/v1/render/png":    {"image/png", "\x89PNG\r\n\x1a\n"},
		"/api/v1/render/mixed":  {"image/png", "\x89PNG"},
		"/api/v1/docs/markdown": {"text/markdown; charset=utf-8", "# Title\n"},
		"/api/v1/docs/pdf":      {"application/pdf", "%PDF-1.7"},
	}
	accepts := map[string]string{}
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		accepts[r.URL.Path] = r.Header.Get("Accept")
		b := bodies[r.URL.Path]
		w.Header().Set("Content-Type", b[0])
		_, _ = io.WriteString(w, b[1])
	})
	ctx := context.Background()
	results := map[string]func() (*Content, error){
		"/api/v1/render/svg":    func() (*Content, error) { return client.Render.SVG(ctx, api.RenderRequest{Size: 500}) },
		"/api/v1/render/png":    func() (*Content, error) { return client.Render.PNG(ctx) },
		"/api/v1/render/mixed":  func() (*Content, error) { return client.Render.Mixed(ctx, api.RenderRequest{Size: 500}) },
		"/api/v1/docs/markdown": func() (*Content, error) { return client.Docs.Markdown(ctx) },
		"/api/v1/docs/pdf":      func() (*Content, error) { return client.Docs.PDF(ctx, &api.V1DocsPdfParams{JobID: "j"}) },
	}
	wantAccept := map[string]string{
		"/api/v1/render/svg":    "image/svg+xml",
		"/api/v1/render/png":    "image/png",
		"/api/v1/render/mixed":  "image/png, image/svg+xml",
		"/api/v1/docs/markdown": "text/markdown",
		"/api/v1/docs/pdf":      "application/pdf",
	}
	for path, call := range results {
		content, err := call()
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if content.ContentType != bodies[path][0] || string(content.Data) != bodies[path][1] {
			t.Errorf("%s: %q %q", path, content.ContentType, content.Data)
		}
		if accepts[path] != wantAccept[path] {
			t.Errorf("%s: accept %q, want %q", path, accepts[path], wantAccept[path])
		}
	}
}
