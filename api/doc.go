// Package api holds the request and response types of FreeAstroAPI and a low-level client,
// generated from spec/freeastro.json by oapi-codegen (see scripts/generate.sh).
//
// Most programs use these types with the higher-level client in package
// github.com/GabrielRw/freeastroapi-go-sdk, which adds authentication, retries and typed
// errors. The low-level Client and ClientWithResponses are exported as well but do none of
// that by themselves.
//
// Optional fields are pointers; freeastro.Ptr helps build them.
package api

import (
	"bytes"
	"encoding/json"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// DateFormat is the layout of a Date on the wire.
const DateFormat = openapi_types.DateFormat

// Date is a calendar date (format "date" in the OpenAPI document), such as a path
// parameter of /api/v1/chinese/calendar/{date}:
//
//	api.Date{Time: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)}
//
// It is sent as "2006-01-02". Decoding is lenient so that a response never fails because of
// a date: null and "" leave the zero Date, and a full RFC 3339 timestamp is accepted too.
// Date converts to and from openapi_types.Date.
type Date struct {
	time.Time
}

// MarshalJSON encodes the date as "2006-01-02".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(DateFormat)) }

// UnmarshalJSON accepts "2006-01-02", an RFC 3339 timestamp, "" or null.
func (d *Date) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return d.UnmarshalText([]byte(s))
}

// MarshalText encodes the date as "2006-01-02" (used for path and query parameters).
func (d Date) MarshalText() ([]byte, error) { return []byte(d.Format(DateFormat)), nil }

// UnmarshalText accepts "2006-01-02", an RFC 3339 timestamp or "".
func (d *Date) UnmarshalText(data []byte) error {
	s := string(bytes.TrimSpace(data))
	if s == "" {
		*d = Date{}
		return nil
	}
	t, err := time.Parse(DateFormat, s)
	if err != nil {
		var err2 error
		if t, err2 = time.Parse(time.RFC3339Nano, s); err2 != nil {
			return err
		}
	}
	d.Time = t
	return nil
}

// Bind implements the oapi-codegen runtime Binder interface, so a Date is bound as a
// scalar query or path parameter.
func (d *Date) Bind(src string) error { return d.UnmarshalText([]byte(src)) }

// String returns the date as "2006-01-02".
func (d Date) String() string { return d.Format(DateFormat) }

// UUID is a value of format "uuid".
type UUID = openapi_types.UUID

// Email is a value of format "email".
type Email = openapi_types.Email

// File is a file upload or download.
type File = openapi_types.File
