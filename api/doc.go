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

import openapi_types "github.com/oapi-codegen/runtime/types"

// Date is a calendar date (format "date" in the OpenAPI document), such as a path
// parameter of /api/v1/chinese/calendar/{date}:
//
//	api.Date{Time: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)}
type Date = openapi_types.Date

// UUID is a value of format "uuid".
type UUID = openapi_types.UUID

// Email is a value of format "email".
type Email = openapi_types.Email

// File is a file upload or download.
type File = openapi_types.File
