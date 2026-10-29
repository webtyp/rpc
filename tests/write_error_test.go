package rpc_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/rpc"
	"webtyp.com/server/httpd"
)

type errorTestModule struct{}

func (errorTestModule) ModelName() string { return "errormod" }
func (errorTestModule) MountOperations(reg router.OperationRegistry) {
	reg.Operation("not_found", func(c router.Context) {
		rpc.WriteError(c, model.NotFound("service not found"))
	}).Public()
	reg.Operation("conflict", func(c router.Context) {
		rpc.WriteError(c, model.Conflict("in use"))
	}).Public()
	reg.Operation("field_error", func(c router.Context) {
		rpc.WriteError(c, &model.FieldError{Field: "name", Err: fmt.Err("name required")})
	}).Public()
	reg.Operation("internal", func(c router.Context) {
		rpc.WriteError(c, fmt.Err("pq: relation x does not exist"))
	}).Public()
	reg.Operation("plain", func(c router.Context) {
		c.WriteStatus(400)
		c.Write([]byte("bad"))
	}).Public()
}

func setupErrorTestServer(t *testing.T, log func(...any)) (*httptest.Server, router.Router) {
	mux := http.NewServeMux()
	r := httpd.NewRouter(mux)
	if err := rpc.Mount(r, rpc.DefaultPrefix, log, errorTestModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	ts := httptest.NewServer(mux)
	return ts, r
}

func call(caller *rpc.Caller, op string) error {
	doneCh := make(chan error, 1)
	caller.Call(op, nil, nil, func(err error) {
		doneCh <- err
	})
	select {
	case err := <-doneCh:
		return err
	case <-time.After(1 * time.Second):
		return fmt.Err("timeout")
	}
}

func getRPCError(t *testing.T, err error) *rpc.Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := err.(*rpc.Error)
	if !ok {
		t.Fatalf("expected *rpc.Error, got %T: %v", err, err)
	}
	return e
}

func TestWriteError_NotFound(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.not_found")
	e := getRPCError(t, err)

	if e.Status != 404 {
		t.Errorf("expected status 404, got %d", e.Status)
	}
	if e.Detail != "service not found" {
		t.Errorf("expected Detail 'service not found', got %q", e.Detail)
	}
	if e.Field != "" {
		t.Errorf("expected Field '', got %q", e.Field)
	}
}

func TestWriteError_Conflict(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.conflict")
	e := getRPCError(t, err)

	if e.Status != 409 {
		t.Errorf("expected status 409, got %d", e.Status)
	}
}

func TestWriteError_FieldError(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.field_error")
	e := getRPCError(t, err)

	if e.Status != 400 {
		t.Errorf("expected status 400, got %d", e.Status)
	}
	if e.Field != "name" {
		t.Errorf("expected Field 'name', got %q", e.Field)
	}
	if e.Error() != "rpc: 400 name required" {
		t.Errorf("expected Error() 'rpc: 400 name required', got %q", e.Error())
	}
}

func TestWriteError_InternalHidesDetail(t *testing.T) {
	var logBuf bytes.Buffer
	logFn := func(args ...any) {
		for i, a := range args {
			if i > 0 {
				logBuf.WriteString(" ")
			}
			logBuf.WriteString(fmt.Sprintf("%v", a))
		}
		logBuf.WriteString("\n")
	}

	ts, _ := setupErrorTestServer(t, logFn)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.internal")
	e := getRPCError(t, err)

	if e.Status != 500 {
		t.Errorf("expected status 500, got %d", e.Status)
	}
	if strings.Contains(e.Body, "pq:") {
		t.Errorf("expected body to hide 'pq:', got %q", e.Body)
	}
	if e.Detail != "internal error" {
		t.Errorf("expected Detail 'internal error', got %q", e.Detail)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "pq: relation x does not exist") {
		t.Errorf("expected log to contain 'pq: relation x does not exist', got %q", logOutput)
	}
}

func TestWriteError_ProblemContentType(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL+"/api/errormod/not_found", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("expected Content-Type 'application/problem+json', got %q", ct)
	}
}

func TestCaller_PlainBodyStillWorks(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.plain")
	e := getRPCError(t, err)

	if e.Status != 400 {
		t.Errorf("expected status 400, got %d", e.Status)
	}
	if e.Body != "bad" {
		t.Errorf("expected Body 'bad', got %q", e.Body)
	}
	if e.Detail != "" {
		t.Errorf("expected Detail '', got %q", e.Detail)
	}
	if e.Error() != "rpc: 400 bad" {
		t.Errorf("expected Error() 'rpc: 400 bad', got %q", e.Error())
	}
}

// The browser asks the same two questions the server does: model.KindOf and
// model.FieldOf answer the rpc client's error, so a screen never needs to know
// the transport.
func TestCaller_ErrorAnswersKindOfAndFieldOf(t *testing.T) {
	ts, _ := setupErrorTestServer(t, nil)
	defer ts.Close()
	caller := rpc.NewCaller(ts.URL)

	err := call(caller, "errormod.field_error")
	if got := model.KindOf(err); got != model.ErrorInvalid {
		t.Errorf("KindOf(field error) = %v, want %v", got, model.ErrorInvalid)
	}
	if got := model.FieldOf(err); got != "name" {
		t.Errorf("FieldOf(field error) = %q, want %q", got, "name")
	}

	err = call(caller, "errormod.not_found")
	if got := model.KindOf(err); got != model.ErrorNotFound {
		t.Errorf("KindOf(not found) = %v, want %v", got, model.ErrorNotFound)
	}
}
