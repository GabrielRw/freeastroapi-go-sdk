package freeastro

// Tests for an SDK generated from testdata/fixtures/extra-spec.json, which covers generator
// paths specific to the Go SDK. Run by scripts/test-fixture.sh.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

func extraClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := NewClient(WithAPIKey("k"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestExtraUntypedThenTyped(t *testing.T) {
	var gotKey, gotQuery string
	client := extraClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotQuery = r.Header.Get("Idempotency-Key"), r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"name":"Sun","longitude":84.123456789,"kind":"item","note":null}],"total":1}`)
	})
	ctx := context.Background()

	raw, err := client.Items.ListV1(ctx, &api.V1ItemsListParams{Limit: Ptr(1), IdempotencyKey: Ptr("k-1")})
	if err != nil || !json.Valid(raw) || gotKey != "k-1" || gotQuery != "limit=1" {
		t.Fatalf("ListV1 %s %v key=%q query=%q", raw, err, gotKey, gotQuery)
	}
	list, err := client.Items.List(ctx, nil)
	if err != nil || list.Total != 1 || list.Items[0].Longitude != 84.123456789 || list.Items[0].Kind != "item" || list.Items[0].Note != nil {
		t.Fatalf("List %+v %v", list, err)
	}
}

func TestExtraRangeErrorResponses(t *testing.T) {
	client := extraClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"detail":"maintenance","code":"E_DOWN"}`)
	})
	_, err := client.Items.List(context.Background(), nil)
	var serverErr *ServerError
	if !errors.As(err, &serverErr) || serverErr.Code != "E_DOWN" || serverErr.Message != "maintenance" {
		t.Fatalf("%#v", err)
	}
}

func TestExtraJSONOrMarkdown(t *testing.T) {
	client := extraClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "markdown") {
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = io.WriteString(w, "# Report\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"headline":"Good","score":0.5}`)
	})
	ctx := context.Background()

	summary, err := client.Reports.Summary(ctx, api.SummaryRequest{ReportID: "r1"})
	if err != nil || summary.Headline != "Good" || summary.Score == nil || *summary.Score != 0.5 {
		t.Fatalf("Summary %+v %v", summary, err)
	}
	content, err := client.Reports.SummaryRaw(ctx, api.SummaryRequest{ReportID: "r1"})
	if err != nil || content.MediaType() != "text/markdown" || content.String() != "# Report\n" {
		t.Fatalf("SummaryRaw %+v %v", content, err)
	}
}

func TestExtraNoContentAndRawBody(t *testing.T) {
	var method, path, ctype, body string
	client := extraClient(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		method, path, ctype, body = r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(data)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF")
	})
	ctx := context.Background()

	if err := client.Reports.Delete(ctx, "r1"); err != nil || method != "DELETE" || path != "/api/v1/reports/r1" {
		t.Fatalf("Delete %v %s %s", err, method, path)
	}
	pdf, err := client.Uploads.Text(ctx, "text/plain", strings.NewReader("hello"))
	if err != nil || string(pdf.Data) != "%PDF" || ctype != "text/plain" || body != "hello" {
		t.Fatalf("Text %+v %v %q %q", pdf, err, ctype, body)
	}
}
