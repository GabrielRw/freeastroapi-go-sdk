package freeastro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// This file does not depend on any particular operation, so scripts/test-fixture.sh also
// runs it against SDKs generated from the synthetic fixture specs.

type specOp struct {
	method string
	path   string
}

func loadSpecOperations(t *testing.T) map[string]specOp {
	t.Helper()
	data, err := os.ReadFile("spec/freeastro.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	ops := map[string]specOp{}
	for path, item := range doc.Paths {
		for method, op := range item {
			switch method {
			case "get", "post", "put", "patch", "delete":
				ops[op.OperationID] = specOp{strings.ToUpper(method), path}
			}
		}
	}
	return ops
}

// TestOperationsMatchSpec checks that Operations lists exactly the spec's operations.
func TestOperationsMatchSpec(t *testing.T) {
	spec := loadSpecOperations(t)
	ops := Operations()
	if len(ops) != len(spec) {
		t.Errorf("Operations() has %d entries, spec has %d operations", len(ops), len(spec))
	}
	seen := map[string]bool{}
	for _, op := range ops {
		want, ok := spec[op.OperationID]
		if !ok {
			t.Errorf("%s is not in the spec", op.OperationID)
			continue
		}
		if op.Method != want.method || op.Path != want.path {
			t.Errorf("%s: %s %s, spec says %s %s", op.OperationID, op.Method, op.Path, want.method, want.path)
		}
		accessor := strings.Join(op.Accessor, ".")
		if seen[accessor] {
			t.Errorf("two operations share Client.%s", accessor)
		}
		seen[accessor] = true
	}
	if !sort.SliceIsSorted(ops, func(i, j int) bool { return ops[i].OperationID < ops[j].OperationID }) {
		t.Error("Operations() is not sorted by operationId")
	}
}

// TestEveryOperationIsReachable calls every operation through its Client method against a
// test server and checks that the method sends the right HTTP method and path.
func TestEveryOperationIsReachable(t *testing.T) {
	var mu sync.Mutex
	var got []specOp
	var serveContent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		got = append(got, specOp{r.Method, r.URL.Path})
		content := serveContent
		mu.Unlock()
		if content {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = io.WriteString(w, "<svg/>")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	client, err := NewClient(WithAPIKey("test"), WithBaseURL(srv.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}

	contentType := reflect.TypeOf(&Content{})
	for _, op := range Operations() {
		methods, err := resolveMethods(client, op.Accessor)
		if err != nil {
			t.Errorf("%s: %v", op.OperationID, err)
			continue
		}
		pattern := pathPattern(op.Path)
		for name, method := range methods {
			mu.Lock()
			got = nil
			serveContent = method.Type().NumOut() == 2 && method.Type().Out(0) == contentType
			mu.Unlock()

			out := method.Call(zeroArgs(method.Type()))
			if errV := out[len(out)-1]; !errV.IsNil() {
				t.Errorf("%s (%s): %v", op.OperationID, name, errV.Interface())
				continue
			}
			mu.Lock()
			calls := append([]specOp(nil), got...)
			mu.Unlock()
			if len(calls) != 1 || calls[0].method != op.Method || !pattern.MatchString(calls[0].path) {
				t.Errorf("%s (%s): sent %v, want %s %s", op.OperationID, name, calls, op.Method, op.Path)
			}
		}
	}
}

// resolveMethods follows an accessor such as ["Western", "Natal", "Insights"] on the client
// and returns that method, plus its Raw variant when there is one.
func resolveMethods(client *Client, accessor []string) (map[string]reflect.Value, error) {
	v := reflect.ValueOf(client)
	for _, field := range accessor[:len(accessor)-1] {
		v = v.Elem().FieldByName(field)
		if !v.IsValid() || v.IsNil() {
			return nil, fmt.Errorf("no service Client.%s", strings.Join(accessor, "."))
		}
	}
	name := accessor[len(accessor)-1]
	method := v.MethodByName(name)
	if !method.IsValid() {
		return nil, fmt.Errorf("no method Client.%s", strings.Join(accessor, "."))
	}
	methods := map[string]reflect.Value{name: method}
	if raw := v.MethodByName(name + "Raw"); raw.IsValid() {
		methods[name+"Raw"] = raw
	}
	return methods, nil
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	readerType  = reflect.TypeOf((*io.Reader)(nil)).Elem()
)

// zeroArgs builds arguments for a call: a context, "x" for strings (path parameters and
// content types), a JSON body for readers and zero values for everything else.
func zeroArgs(fn reflect.Type) []reflect.Value {
	n := fn.NumIn()
	if fn.IsVariadic() {
		n--
	}
	args := make([]reflect.Value, n)
	for i := 0; i < n; i++ {
		switch in := fn.In(i); {
		case in == contextType:
			args[i] = reflect.ValueOf(context.Background())
		case in == readerType:
			args[i] = reflect.ValueOf(strings.NewReader("{}"))
		case in.Kind() == reflect.String:
			args[i] = reflect.ValueOf("x").Convert(in)
		default:
			args[i] = reflect.Zero(in)
		}
	}
	return args
}

func pathPattern(path string) *regexp.Regexp {
	parts := regexp.MustCompile(`\{[^}]+\}`).Split(path, -1)
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile("^" + strings.Join(parts, "[^/]+") + "$")
}
