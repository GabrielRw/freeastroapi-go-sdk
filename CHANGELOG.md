# Changelog

## 0.1.0

- First release of the Go SDK, `github.com/GabrielRw/freeastroapi-go-sdk` (package `freeastro`).
- Cover every operation in the public FreeAstroAPI OpenAPI document (142 operations), grouped
  by URL path as in the TypeScript and Python SDKs; the latest API version gets the plain method
  name and older versions a `V1`/`V2` suffix. `Operations()` lists every operation.
- Request and response types in package `api`, generated with oapi-codegen v2.8.2 from the
  vendored spec (`spec/freeastro.json`, 142 operations, 975 schemas). Every operation has a
  typed response: the 130 JSON operations return `*api.T`; images (SVG, PNG) and PDF are
  returned as `*Content`, with a `ReportRaw` variant for the JSON-or-PDF report job.
- Lenient response decoding: a missing required field, a `null` in a non-null field (dates
  included, through `api.Date`), an unknown field or an unknown enum value never makes a call
  fail. Request types keep their required fields.
- `NewClient` with functional options: API key (falls back to `FREEASTRO_API_KEY`), base URL,
  HTTP client, per-attempt timeout, retries, headers and User-Agent (`freeastroapi-go/0.1.0`).
- The retry policy shared by every FreeAstroAPI SDK: 2 retries by default, exponential backoff
  from 0.5 s (factor 2, full jitter, 8 s cap). 429 is retried for every operation, honouring
  `Retry-After` and `retry_after_ms` (a wait over 30 s returns the `RateLimitError` at once);
  408, 5xx, network errors and timeouts are retried for every operation except the
  side-effecting ones (`SideEffectingOperations()`), which need an idempotency key or
  `WithRetry(true)`. Other 4xx are never retried. Per call: `WithIdempotencyKey`, `WithRetry`
  and `WithRequestMaxRetries`.
- Typed errors: `APIError`, `AuthenticationError`, `PermissionError`, `NotFoundError`,
  `ConflictError`, `ValidationError`, `RateLimitError`, `ServerError`, `NetworkError`,
  `TimeoutError` and `DecodeError`. Error bodies follow `api.ErrorResponse`
  (`APIError.ErrorResponse()`); `Code` carries the stable error code and
  `RateLimitError.RetryAfter` the server's requested wait.
- `Client.Do` escape hatch for endpoints newer than the SDK.
- CI (tests on Go 1.24 and 1.26, generated-code check, synthetic spec fixtures, Prism contract
  tests), tag-triggered releases, a daily spec sync pull request and a daily live smoke test.
