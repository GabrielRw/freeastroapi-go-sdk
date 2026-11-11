# Changelog

## 0.1.0

- First release of the Go SDK, `github.com/GabrielRw/freeastroapi-go-sdk` (package `freeastro`).
- Cover every operation in the public FreeAstroAPI OpenAPI document (142 operations), grouped
  by URL path as in the TypeScript and Python SDKs; the latest API version gets the plain method
  name and older versions a `V1`/`V2` suffix. `Operations()` lists every operation.
- Request and response types in package `api`, generated with oapi-codegen v2.8.2 from the
  vendored spec (`spec/freeastro.json`). Undocumented responses are returned as
  `json.RawMessage`; images, PDF and Markdown as `*Content`.
- `NewClient` with functional options: API key (falls back to `FREEASTRO_API_KEY`), base URL,
  HTTP client, per-attempt timeout, retries, headers and User-Agent (`freeastroapi-go/0.1.0`).
- Retries with capped exponential backoff and `Retry-After` for GET requests and for requests
  with an idempotency key or `WithRetry(true)`.
- Typed errors: `APIError`, `AuthenticationError`, `PermissionError`, `NotFoundError`,
  `ConflictError`, `ValidationError`, `RateLimitError`, `ServerError`, `NetworkError`,
  `TimeoutError` and `DecodeError`.
- `Client.Do` escape hatch for endpoints newer than the SDK.
- CI (tests on Go 1.24 and 1.26, generated-code check, synthetic spec fixtures, Prism contract
  tests), tag-triggered releases, a daily spec sync pull request and a daily live smoke test.
