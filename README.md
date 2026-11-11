# FreeAstroAPI Go SDK

[![CI](https://github.com/GabrielRw/freeastroapi-go-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/GabrielRw/freeastroapi-go-sdk/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/GabrielRw/freeastroapi-go-sdk.svg)](https://pkg.go.dev/github.com/GabrielRw/freeastroapi-go-sdk)

Official Go client for [FreeAstroAPI](https://www.freeastroapi.com): Western, Vedic and Chinese
astrology, numerology, horoscopes, ephemeris, geo search and more. Every operation of the
[public OpenAPI document](spec/freeastro.json) (142 operations) has a typed method.

API reference: <https://www.freeastroapi.com/docs>

## Install

```sh
go get github.com/GabrielRw/freeastroapi-go-sdk
```

Requires Go 1.24 or later. The SDK is pre-1.0: minor versions may contain breaking changes.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	freeastro "github.com/GabrielRw/freeastroapi-go-sdk"
	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

func main() {
	// Reads the key from FREEASTRO_API_KEY; or pass freeastro.WithAPIKey("...").
	client, err := freeastro.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	cities, err := client.Geo.Search(context.Background(), &api.V2GeoSearchParams{
		Q:     "Paris",
		Limit: freeastro.Ptr(3),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(cities.Results[0].Timezone) // Europe/Paris
}
```

Keep API keys on servers and other trusted runtimes.

Package `freeastro` holds the client; package `api` holds the request and response types,
generated from the OpenAPI document. Optional fields are pointers: `freeastro.Ptr(v)` builds one.

## Examples

### Natal chart

```go
chart, err := client.Natal.Calculate(ctx, nil, api.NatalChartRequest{
	Year: 1990, Month: 6, Day: 15,
	Hour: freeastro.Ptr(14), Minute: freeastro.Ptr(30),
	City: freeastro.Ptr("Paris"),
})
if err != nil {
	log.Fatal(err)
}
// chart is *api.NatalCalculateResponse; optional fields are pointers.
if chart.Planets != nil {
	for _, p := range *chart.Planets {
		fmt.Println(p.Name, p.Sign)
	}
}
```

Every JSON response is decoded into its type, for example the Sun sign:

```go
sun, err := client.Western.Signs.Sun(ctx, api.NatalChartRequest{
	Year: 1990, Month: 6, Day: 15, City: freeastro.Ptr("Paris"),
})
fmt.Println(sun.Sign, sun.Degree, sun.House) // *api.SignResponse
```

### Vedic panchang

```go
panchang, err := client.Vedic.Panchang(ctx, api.PanchangRequestV2{
	Year: 2026, Month: 1, Day: 14, Hour: 6, Minute: 0,
	City: freeastro.Ptr("Delhi"),
})
// panchang is *api.PanchangV2Response
```

### Geo search

```go
cities, err := client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "Jingzhou, Hubei", Country: freeastro.Ptr("CN")})
for _, c := range cities.Results {
	fmt.Println(c.Name, c.Lat, c.Lng, c.Timezone)
}
```

### Charts as SVG or PNG

Operations that return images or documents return `*freeastro.Content` (content type and bytes):

```go
svg, err := client.Vedic.KPRender(ctx, api.KPChartRenderRequest{
	Year: 1990, Month: 6, Day: 15, Hour: 14, Minute: 30, City: freeastro.Ptr("Paris"),
})
os.WriteFile("kp.svg", svg.Data, 0o644) // svg.ContentType == "image/svg+xml"
```

## Resources and method names

Methods are grouped by URL path, exactly like the TypeScript and Python SDKs:

| Call | Endpoint |
| --- | --- |
| `client.Natal.Calculate(ctx, params, body)` | `POST /api/v1/natal/calculate` |
| `client.Western.Natal.Insights(ctx, body)` | `POST /api/v1/western/natal/insights` |
| `client.Western.Progressions.SecondaryExactAspects(ctx, body)` | `POST /api/v1/western/progressions/secondary/exact-aspects` |
| `client.Vedic.Panchang(ctx, body)` | `POST /api/v2/vedic/panchang` |
| `client.Chinese.Calendar(ctx, date, params)` | `GET /api/v1/chinese/calendar/{date}` |
| `client.Horoscope.Daily.Sign(ctx, params)` | `GET /api/v3/horoscope/daily/sign` |

When an operation exists in several API versions, the plain name calls the latest version and
older versions carry a suffix: `client.Vedic.Chart` calls `/api/v2/vedic/chart`,
`client.Vedic.ChartV1` calls `/api/v1/vedic/chart`, and `client.Horoscope.Daily.SignV1` /
`SignV2` reach the older daily horoscopes.

Arguments are always `ctx`, then path parameters, then the query parameters struct
(`*api.<Operation>Params`, may be nil when nothing is required), then the JSON body, then
per-request options. Resource groups: `AI`, `Chinese`, `Ephemeris`, `Geo`, `Horoscope`, `Moon`,
`Natal`, `Numerology`, `SkyEvents`, `Space`, `Transits`, `Vedic` and `Western`.
`freeastro.Operations()` lists every operation with its method.

Return types follow the spec: every JSON response is a typed `*api.T`, and images, PDF and
Markdown are `*freeastro.Content`. Operations that can answer with JSON or a file (the natal
report job) also have a `...Raw` variant returning `*freeastro.Content`.

Response decoding is lenient, because some response schemas were inferred from samples: a
missing required field or a `null` leaves the Go zero value, unknown fields are ignored (or
kept in `AdditionalProperties` where the schema allows extra fields), and an enum field holds
any string the API sends, so an unknown value never makes a call fail. Request types keep
their required fields.

For newer endpoints use the escape hatch:

```go
var out map[string]any
err := client.Do(ctx, "POST", "/api/v1/example", nil, map[string]any{"a": 1}, &out)
```

## Errors

HTTP errors are typed. Each wraps an `*freeastro.APIError` with `StatusCode`, `Message`,
`Code` (the stable error code such as `rate_limited`), `RequestID`, `Body` and `Header`;
`apiErr.ErrorResponse()` decodes the body as the documented `*api.ErrorResponse`:

| Type | Status |
| --- | --- |
| `*AuthenticationError` | 401 |
| `*PermissionError` | 403 |
| `*NotFoundError` | 404 |
| `*ConflictError` | 409 |
| `*ValidationError` | 400, 422 |
| `*RateLimitError` | 429 (with `RetryAfter`) |
| `*ServerError` | 5xx |

```go
_, err := client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "Paris"})

var rateLimited *freeastro.RateLimitError
var apiErr *freeastro.APIError
switch {
case errors.As(err, &rateLimited):
	time.Sleep(rateLimited.RetryAfter)
case errors.As(err, &apiErr):
	log.Println(apiErr.StatusCode, apiErr.Code, apiErr.RequestID)
}
```

Failures to reach the API are `*freeastro.NetworkError`; a per-attempt timeout is a
`*freeastro.TimeoutError` (which also matches `*NetworkError`). When your own context is
cancelled or expires, the context's error is returned.

## Retries and timeouts

Every SDK for FreeAstroAPI uses the same retry policy:

- Up to 2 retries (`WithMaxRetries`), with exponential backoff and full jitter: retry *n*
  waits a random time between 0 and 0.5 s × 2^*n*, capped at 8 s (`WithRetryDelay`).
- **429** is retried for every operation, since the request was rejected before it ran. The
  wait comes from the `Retry-After` header (seconds or an HTTP date) or `retry_after_ms` in the
  body, else the backoff. When the server asks for more than 30 s, the SDK does not wait: it
  returns the `*RateLimitError` at once, with `RetryAfter` set.
- **408, 500, 502, 503, 504, network errors and timeouts** are retried for every operation
  except the side-effecting ones, `freeastro.SideEffectingOperations()`:
  `v1_natal_calculate` (may create a paid report job), `v1_ai_generate`, `v2_vedic_chat`,
  `v2_western_chat`, `v2_vedic_qa`, `v1_western_transits_timeline` and
  `v1_western_transits_insights`. These are retried only when the call has an idempotency key
  (sent as the `Idempotency-Key` header) or opts in with `WithRetry(true)`.
- Other 4xx responses are never retried.

```go
client, err := freeastro.NewClient(
	freeastro.WithTimeout(30*time.Second),                  // per attempt, default 20s
	freeastro.WithMaxRetries(3),                            // default 2, 0 disables
	freeastro.WithRetryDelay(time.Second, 10*time.Second),  // default 500ms, 8s
)

reply, err := client.Vedic.Chat(ctx, nil, body, freeastro.WithIdempotencyKey("stable-key"))
reply, err = client.Vedic.Chat(ctx, nil, body, freeastro.WithRetry(true))     // opt in
sun, err := client.Western.Signs.Sun(ctx, req, freeastro.WithRetry(false))   // no retries at all
sun, err = client.Western.Signs.Sun(ctx, req, freeastro.WithRequestMaxRetries(5))
```

`Client.Do` applies the policy of the operation matching its method and path.

Other options: `WithBaseURL`, `WithHTTPClient`, `WithHeader`, `WithUserAgent`, and per request
`WithRequestTimeout`, `WithRequestHeader` and `WithRawResponse` (to read response headers).
Requests carry `User-Agent: freeastroapi-go/<version>`.

## Development

Go does not need to be installed: the scripts run in the pinned `golang:1.26.11` Docker image
(set `FREEASTRO_NATIVE=1` to use a local Go toolchain instead).

```sh
scripts/generate.sh                      # regenerate from spec/freeastro.json
scripts/generate.sh --check              # fail if generated files are stale (CI)
scripts/docker.sh -- go test ./...       # run any Go command in Docker
scripts/test-fixture.sh                  # generator checks on the synthetic specs
python3 scripts/spec_sync.py fetch       # update the vendored spec from production
python3 scripts/with_prism.py -- go test -run TestContract -count=1 .   # Prism contract tests
```

Generation has three pinned steps:

1. `internal/cmd/specprep` applies three deterministic rewrites to a copy of the spec: it drops
   validation-only unions (`anyOf: [{required: [city]}, {required: [lat, lng]}]`, which would
   otherwise turn requests into union types), gives `const` schemas without a `type` the type
   of their value, and drops `null` from `additionalProperties` unions of several types (which
   oapi-codegen v2.8 would otherwise generate as code that does not compile). No OpenAPI 3.1 to
   3.0 downgrade is needed. Format `date` maps to `api.Date`, which decodes leniently.
2. [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2.8.2 (pinned in
   `internal/tools/go.mod`, configured in `internal/codegen/oapi-codegen.yaml`) writes the types
   and a low-level client to `api/api.gen.go`.
3. `internal/cmd/genresources` writes the resource layer (`resources_gen.go`) and the operation
   table (`operations_gen.go`).

The tests cover the transport (auth, retries, errors), lenient decoding of every response type
(`decode_test.go`), every operation through its method (`coverage_test.go`), the generator on synthetic specs with typed and untyped responses,
`ErrorResponse` bodies and SVG/PNG/PDF/Markdown content (`scripts/test-fixture.sh`), every
operation against a Prism mock (`contract_test.go`, when `PRISM_URL` is set) and five read-only
production calls (`live_test.go`, when `FREEASTRO_TEST_API_KEY` is set).

The `Sync spec` workflow fetches the production spec daily and opens a pull request from
`spec-sync/auto` when it changes.

## Releasing

Bump `Version` in `freeastro.go`, update `CHANGELOG.md`, then push a matching tag
(`git tag v0.1.0 && git push origin v0.1.0`). The `Release` workflow runs the checks, creates
the GitHub release and asks proxy.golang.org to fetch the version.

Issues and contributions are welcome on
[GitHub](https://github.com/GabrielRw/freeastroapi-go-sdk).

Licensed under the Apache License 2.0.
