package freeastro

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GabrielRw/freeastroapi-go-sdk/api"
)

// Response "required" lists in the spec were partly inferred from samples, so decoding must
// tolerate missing required fields, nulls in non-null fields, unknown fields and unknown
// enum values.

func TestLenientDecoding(t *testing.T) {
	srv := newServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// "timestamp" (required) is missing, "next_phases" (required, non-null) is null,
		// "phase.name" has a value outside its enum, "is_waxing" is null and "zz_new" is
		// unknown.
		_, _ = w.Write([]byte(`{
			"next_phases": null,
			"phase": {"name": "Hybrid Moon", "illumination": 0.5, "is_waxing": null},
			"zz_new": {"nested": [1, 2]}
		}`))
	})
	client, _ := newTestClient(t, srv)
	moon, err := client.Moon.Phase(context.Background(), nil)
	if err != nil {
		t.Fatalf("lenient decode failed: %v", err)
	}
	if moon.Phase.Name != "Hybrid Moon" || moon.Phase.Illumination != 0.5 || moon.Timestamp != "" || !moon.NextPhases.FullMoon.IsZero() {
		t.Fatalf("decoded %+v", moon)
	}
}

func TestLenientDates(t *testing.T) {
	var row struct {
		A api.Date  `json:"a"`
		B api.Date  `json:"b"`
		C api.Date  `json:"c"`
		D *api.Date `json:"d"`
		E api.Date  `json:"e"`
	}
	in := `{"a": null, "b": "", "c": "2026-02-17", "d": null, "e": "2026-02-17T10:00:00Z"}`
	if err := json.Unmarshal([]byte(in), &row); err != nil {
		t.Fatal(err)
	}
	if !row.A.IsZero() || !row.B.IsZero() || row.C.String() != "2026-02-17" || row.D != nil || row.E.String() != "2026-02-17" {
		t.Fatalf("%+v", row)
	}
	out, err := json.Marshal(api.Date{Time: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)})
	if err != nil || string(out) != `"2026-02-17"` {
		t.Fatalf("marshal %s %v", out, err)
	}
}

// TestEveryResponseTypeIsLenient decodes, into the success type of every operation,
// documents built from that type in which every field is missing, every leaf is null, or
// every string holds an unknown value, with an unknown field in every object.
func TestEveryResponseTypeIsLenient(t *testing.T) {
	types := responseTypes(t)
	if len(types) < 100 {
		t.Fatalf("only %d response types found", len(types))
	}
	t.Logf("checking %d response types", len(types))
	for name, typ := range types {
		docs := map[string]any{
			"empty":   map[string]any{},
			"null":    nil,
			"nulls":   sampleFor(typ, false, 0),
			"unknown": sampleFor(typ, true, 0),
		}
		for variant, doc := range docs {
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, variant, err)
			}
			if err := json.Unmarshal(data, reflect.New(typ).Interface()); err != nil {
				t.Errorf("%s (%s): %v\n%.400s", name, variant, err, data)
			}
		}
	}
}

// responseTypes returns the 2xx JSON type of every low-level <Op>WithResponse method.
func responseTypes(t *testing.T) map[string]reflect.Type {
	t.Helper()
	out := map[string]reflect.Type{}
	client := reflect.TypeOf(&api.ClientWithResponses{})
	for i := 0; i < client.NumMethod(); i++ {
		m := client.Method(i)
		if !strings.HasSuffix(m.Name, "WithResponse") || strings.HasSuffix(m.Name, "WithBodyWithResponse") {
			continue
		}
		resp := m.Type.Out(0).Elem()
		for j := 0; j < resp.NumField(); j++ {
			f := resp.Field(j)
			if strings.HasPrefix(f.Name, "JSON2") && f.Type.Kind() == reflect.Pointer {
				out[m.Name+"."+f.Name] = f.Type.Elem()
			}
		}
	}
	return out
}

var (
	timeType = reflect.TypeOf(time.Time{})
	dateType = reflect.TypeOf(api.Date{})
)

// sampleFor builds a JSON-ready value for t: objects for structs (with an extra unknown
// field), one-element arrays and maps, and null leaves, or unknown strings and zero numbers
// when unknown is set. Recursion stops after a few levels.
func sampleFor(t reflect.Type, unknown bool, depth int) any {
	if depth > 6 {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return sampleFor(t.Elem(), unknown, depth)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil // json.RawMessage, []byte
		}
		return []any{sampleFor(t.Elem(), unknown, depth+1)}
	case reflect.Map:
		return map[string]any{"key": sampleFor(t.Elem(), unknown, depth+1)}
	case reflect.Struct:
		if t == timeType || t == dateType {
			return nil
		}
		obj := map[string]any{}
		exported := false
		var extra reflect.Type
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			exported = true
			if f.Name == "AdditionalProperties" {
				extra = f.Type.Elem()
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			obj[name] = sampleFor(f.Type, unknown, depth+1)
		}
		if !exported {
			return nil // union types keep the raw JSON
		}
		// An unknown field: free-form unless additionalProperties has a schema.
		if extra == nil || extra.Kind() == reflect.Interface {
			obj["zz_unknown_field"] = map[string]any{"nested": []any{1, "x", nil}}
		} else {
			obj["zz_unknown_field"] = sampleFor(extra, unknown, depth+1)
		}
		return obj
	case reflect.String:
		if unknown {
			return "zz_unknown_enum_value"
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		if unknown {
			return 0
		}
	case reflect.Bool:
		if unknown {
			return false
		}
	}
	return nil
}
