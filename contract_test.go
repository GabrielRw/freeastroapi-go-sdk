package freeastro

// Contract tests: every Client method against a Stoplight Prism mock of spec/freeastro.json.
//
// Prism validates each request (path, query, headers, JSON body) against the spec and rejects
// mismatches, so a passing run means the SDK builds requests the API accepts; the response is
// decoded into the method's return type. Request values are the minimal valid requests printed
// by `scripts/spec_sync.py samples`.
//
//	python3 scripts/with_prism.py -- go test -run TestContract -count=1 ./...
//	PRISM_URL=http://127.0.0.1:4010 go test -run TestContract ./...   # Prism already running

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type requestSample struct {
	PathParams map[string]json.RawMessage `json:"path_params"`
	Query      map[string]json.RawMessage `json:"query"`
	Body       json.RawMessage            `json:"body"`
}

func TestContract(t *testing.T) {
	prismURL := os.Getenv("PRISM_URL")
	if prismURL == "" {
		t.Skip("set PRISM_URL (or run scripts/with_prism.py) for contract tests")
	}
	// FREEASTRO_SAMPLES may name a file holding the samples output, for environments
	// without Python.
	var out []byte
	var err error
	if file := os.Getenv("FREEASTRO_SAMPLES"); file != "" {
		out, err = os.ReadFile(file)
	} else {
		out, err = exec.Command("python3", "scripts/spec_sync.py", "samples", "spec/freeastro.json").Output()
	}
	if err != nil {
		t.Fatalf("request samples: %v", err)
	}
	var samples map[string]requestSample
	if err := json.Unmarshal(out, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != len(Operations()) {
		t.Fatalf("%d samples for %d operations", len(samples), len(Operations()))
	}

	client, err := NewClient(WithAPIKey("contract-test-key"), WithBaseURL(prismURL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range Operations() {
		t.Run(strings.Join(op.Accessor, "."), func(t *testing.T) {
			sample, ok := samples[op.OperationID]
			if !ok {
				t.Fatalf("no sample for %s", op.OperationID)
			}
			methods, err := resolveMethods(client, op.Accessor)
			if err != nil {
				t.Fatal(err)
			}
			for name, method := range methods {
				args, err := sampleArgs(method.Type(), op.Path, sample)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				var resp *http.Response
				args = append(args, reflect.ValueOf(WithRawResponse(&resp)))
				results := method.Call(args)
				if errV := results[len(results)-1]; !errV.IsNil() {
					checkContractError(t, op, name, errV.Interface().(error))
					continue
				}
				var violations []struct {
					Location []any  `json:"location"`
					Message  string `json:"message"`
				}
				_ = json.Unmarshal([]byte(resp.Header.Get("sl-violations")), &violations)
				for _, v := range violations {
					if len(v.Location) > 0 && v.Location[0] == "request" {
						t.Errorf("%s: request violation %v: %s", name, v.Location, v.Message)
					}
				}
			}
		})
	}
}

func checkContractError(t *testing.T, op OperationInfo, name string, err error) {
	t.Helper()
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// Prism validates the request, then cannot serialise schemaless image bodies
		// (image/svg+xml, image/png without a schema or example) and answers 500.
		if apiErr.StatusCode == 500 && bytes.Contains(apiErr.Body, []byte("Cannot find serializer")) {
			return
		}
		t.Errorf("%s %s (%s): %v\nviolations: %s\nbody: %s", op.Method, op.Path, name, err, apiErr.Header.Get("sl-violations"), apiErr.Body)
		return
	}
	t.Errorf("%s %s (%s): %v", op.Method, op.Path, name, err)
}

var pathParamName = regexp.MustCompile(`\{([^}]+)\}`)

// sampleArgs maps a request sample onto a method's parameters: context, path parameters (in
// path order), the *Params struct (query), then the body (or content type and reader).
func sampleArgs(fn reflect.Type, path string, sample requestSample) ([]reflect.Value, error) {
	n := fn.NumIn()
	if fn.IsVariadic() {
		n--
	}
	args := zeroArgs(fn)[:n]
	i := 1
	for _, m := range pathParamName.FindAllStringSubmatch(path, -1) {
		v, err := fromJSON(fn.In(i), sample.PathParams[m[1]])
		if err != nil {
			return nil, err
		}
		args[i] = v
		i++
	}
	if i < n {
		if in := fn.In(i); in.Kind() == reflect.Pointer && strings.HasSuffix(in.Elem().Name(), "Params") {
			query, _ := json.Marshal(sample.Query)
			v, err := fromJSON(in, query)
			if err != nil {
				return nil, err
			}
			args[i] = v
			i++
		}
	}
	body := sample.Body
	if len(body) == 0 {
		body = json.RawMessage("{}")
	}
	switch {
	case i+1 < n && fn.In(i).Kind() == reflect.String && fn.In(i+1) == readerType:
		args[i] = reflect.ValueOf("application/json")
		args[i+1] = reflect.ValueOf(bytes.NewReader(body))
	case i < n:
		v, err := fromJSON(fn.In(i), body)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}

func fromJSON(t reflect.Type, data []byte) (reflect.Value, error) {
	ptr := reflect.New(t)
	if len(data) == 0 {
		return ptr.Elem(), nil
	}
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}
