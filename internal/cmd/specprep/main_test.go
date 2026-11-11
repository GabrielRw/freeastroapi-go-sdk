package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	in := `{
	  "components": {"schemas": {
	    "Req": {
	      "type": "object",
	      "anyOf": [{"required": ["city"]}, {"required": ["lat", "lng"]}],
	      "properties": {
	        "renderer": {"const": "default", "default": "default"},
	        "level": {"const": 2},
	        "ratio": {"const": 0.5},
	        "flag": {"const": true},
	        "typed": {"const": "x", "type": "string"},
	        "example": {"oneOf": [{"type": "string"}, {"type": "integer"}]},
	        "shape": {"anyOf": [{"type": "string"}, {"type": "null"}]}
	      },
	      "example": {"anyOf": [{"required": ["kept"]}]}
	    }
	  }}
	}`
	dec := json.NewDecoder(strings.NewReader(in))
	dec.UseNumber() // as run does
	var doc any
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var s stats
	prepare(doc, &s)
	req := doc.(map[string]any)["components"].(map[string]any)["schemas"].(map[string]any)["Req"].(map[string]any)
	if _, ok := req["anyOf"]; ok {
		t.Error("validation-only anyOf kept")
	}
	if _, ok := req["example"].(map[string]any)["anyOf"]; !ok {
		t.Error("example value rewritten")
	}
	props := req["properties"].(map[string]any)
	types := map[string]string{"renderer": "string", "level": "integer", "ratio": "number", "flag": "boolean", "typed": "string"}
	for name, want := range types {
		if got := props[name].(map[string]any)["type"]; got != want {
			t.Errorf("%s: type %v, want %s", name, got, want)
		}
	}
	if _, ok := props["example"].(map[string]any)["oneOf"]; !ok {
		t.Error("real union in a property named example dropped")
	}
	if !reflect.DeepEqual(props["shape"].(map[string]any)["anyOf"], []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}) {
		t.Error("nullable union changed")
	}
	if s.unions != 1 || s.consts != 4 {
		t.Errorf("stats %+v", s)
	}
}

func TestJSONType(t *testing.T) {
	for v, want := range map[any]string{"s": "string", true: "boolean", json.Number("2"): "integer", json.Number("0.5"): "number"} {
		if got := jsonType(v); got != want {
			t.Errorf("jsonType(%v) = %q, want %q", v, got, want)
		}
	}
}
