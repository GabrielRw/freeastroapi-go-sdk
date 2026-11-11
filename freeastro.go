// Package freeastro is the official Go client for FreeAstroAPI (https://www.freeastroapi.com),
// covering Western, Vedic and Chinese astrology, numerology, horoscopes, ephemeris, geo search
// and more.
//
//	client, err := freeastro.NewClient() // reads FREEASTRO_API_KEY
//	if err != nil {
//		log.Fatal(err)
//	}
//	cities, err := client.Geo.Search(ctx, &api.V2GeoSearchParams{Q: "Paris", Limit: freeastro.Ptr(3)})
//
// Every operation of the public OpenAPI document has a method on Client, grouped by URL path
// (see Client). Request and response types live in package
// github.com/GabrielRw/freeastroapi-go-sdk/api, generated from the same document with
// oapi-codegen.
//
// Every method takes a context.Context first. Errors are typed: see APIError and the types
// that wrap it (AuthenticationError, ValidationError, RateLimitError, ...), NetworkError and
// TimeoutError. Calls are retried (twice by default, with jittered exponential backoff) after
// 429, 408, 500, 502, 503 and 504 responses, network errors and timeouts; a 429 honours
// Retry-After. The few side-effecting operations (SideEffectingOperations) are retried after
// a transient failure only when sent WithIdempotencyKey or WithRetry(true).
package freeastro

// Version is the SDK version, sent in the User-Agent header as freeastroapi-go/<Version>.
const Version = "0.1.0"

// OperationInfo describes one API operation and the Client method that calls it.
type OperationInfo struct {
	// OperationID is the operationId in the OpenAPI document, e.g. "v1_natal_calculate".
	OperationID string
	// Method is the HTTP method, e.g. "POST".
	Method string
	// Path is the URL path template, e.g. "/api/v1/natal/calculate".
	Path string
	// Accessor is the method's path on Client, e.g. ["Natal", "Calculate"].
	Accessor []string
	// Deprecated reports whether the API marks the operation as deprecated.
	Deprecated bool
}

// Operations returns every operation in the OpenAPI document this SDK was generated from,
// sorted by operationId.
func Operations() []OperationInfo {
	out := make([]OperationInfo, len(operations))
	for i, op := range operations {
		op.Accessor = append([]string(nil), op.Accessor...)
		out[i] = op
	}
	return out
}
