// Command specprep prepares the vendored OpenAPI document for oapi-codegen.
//
// oapi-codegen v2.8 reads OpenAPI 3.1 natively, so no 3.1 -> 3.0 downgrade is needed. Two
// deterministic rewrites keep its output correct and idiomatic; they change neither the
// operations nor the wire format:
//
//  1. Validation-only unions are dropped. FastAPI emits object-level constraints such as
//     `anyOf: [{required: [city]}, {required: [lat, lng]}]` ("a city or coordinates"). Every
//     member only adds constraints, so the union describes validation, not shape (the other
//     SDKs ignore it for typing too). oapi-codegen would otherwise turn the request into a
//     union type whose MarshalJSON sends "field": null for every unset untyped field, which
//     the API rejects.
//  2. `const` schemas without a `type` get the type of their value, so a field such as
//     `renderer: {const: "default"}` becomes an optional string enum rather than interface{}.
//
// Usage: specprep IN OUT
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: specprep IN OUT")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "specprep:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	var stats stats
	prepare(doc, &stats)
	result, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(result, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "specprep: dropped %d validation-only unions, typed %d const schemas\n", stats.unions, stats.consts)
	return nil
}

type stats struct{ unions, consts int }

// constraintKeys may appear in a union member that only adds validation.
var constraintKeys = map[string]bool{
	"required": true, "not": true, "title": true, "description": true,
	"minProperties": true, "maxProperties": true,
}

// prepare rewrites schemas in place. Example values are left untouched.
func prepare(node any, s *stats) {
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			prepare(item, s)
		}
	case map[string]any:
		for _, key := range []string{"anyOf", "oneOf"} {
			if members, ok := v[key].([]any); ok && validationOnly(members) {
				delete(v, key)
				s.unions++
			}
		}
		if c, ok := v["const"]; ok {
			if _, typed := v["type"]; !typed {
				if t := jsonType(c); t != "" {
					v["type"] = t
					s.consts++
				}
			}
		}
		for key, child := range v {
			switch key {
			case "example", "examples", "const", "enum":
				continue // values, not schemas
			case "properties":
				// Keys are field names (which may be "example" or "enum"); values are schemas.
				if props, ok := child.(map[string]any); ok {
					for _, schema := range props {
						prepare(schema, s)
					}
					continue
				}
			}
			prepare(child, s)
		}
	}
}

func validationOnly(members []any) bool {
	if len(members) == 0 {
		return false
	}
	for _, m := range members {
		obj, ok := m.(map[string]any)
		if !ok || len(obj) == 0 {
			return false
		}
		for key := range obj {
			if !constraintKeys[key] {
				return false
			}
		}
	}
	return true
}

func jsonType(v any) string {
	switch x := v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if _, err := x.Int64(); err == nil {
			return "integer"
		}
		return "number"
	}
	return ""
}
