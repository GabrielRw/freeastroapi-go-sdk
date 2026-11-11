package freeastro

// Smoke test against production (.github/workflows/live-smoke.yml): five cheap, read-only
// calls. Retries are off so a failure never spends extra quota.
//
//	FREEASTRO_TEST_API_KEY=... go test -run TestLive -count=1 ./...

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

func TestLive(t *testing.T) {
	key := os.Getenv("FREEASTRO_TEST_API_KEY")
	if key == "" {
		t.Skip("set FREEASTRO_TEST_API_KEY to call the production API")
	}
	baseURL := os.Getenv("FREEASTRO_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	client, err := NewClient(WithAPIKey(key), WithBaseURL(baseURL), WithMaxRetries(0), WithTimeout(60*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("geo search", func(t *testing.T) {
		cities, err := client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "Paris", Limit: Ptr(1)})
		if err != nil {
			t.Fatal(err)
		}
		if len(cities.Results) == 0 || cities.Results[0].Timezone == "" {
			t.Fatalf("unexpected result %+v", cities)
		}
	})
	t.Run("moon phase", func(t *testing.T) {
		phase, err := client.Moon.Phase(ctx, &api.V1MoonPhaseParams{Date: Ptr("2026-01-15")})
		if err != nil {
			t.Fatal(err)
		}
		if phase.Phase.Name == "" {
			t.Fatalf("unexpected result %+v", phase)
		}
	})
	t.Run("western sun sign", func(t *testing.T) {
		sign, err := client.Western.Signs.Sun(ctx, api.NatalChartRequest{Year: 1990, Month: 6, Day: 15, City: Ptr("Paris")})
		if err != nil {
			t.Fatal(err)
		}
		if sign.SignID != "gemini" {
			t.Fatalf("sun sign %q, want gemini", sign.SignID)
		}
	})
	t.Run("natal chart", func(t *testing.T) {
		chart, err := client.Natal.Calculate(ctx, nil, api.NatalChartRequest{
			Year: 1990, Month: 6, Day: 15, Hour: Ptr(14), Minute: Ptr(30), City: Ptr("Paris"),
		})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(chart, &fields); err != nil || len(fields) == 0 {
			t.Fatalf("unexpected result %s (%v)", chart, err)
		}
	})
	t.Run("vedic panchang", func(t *testing.T) {
		panchang, err := client.Vedic.Panchang(ctx, api.PanchangRequestV2{
			Year: 2026, Month: 1, Day: 14, Hour: 6, Minute: 0, City: Ptr("Delhi"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if panchang == nil {
			t.Fatal("empty panchang")
		}
	})
}
