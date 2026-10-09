package rpc_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/server/httpd"

	"webtyp.com/rpc"
)

type mockArgs struct {
	Msg string
}

func (m *mockArgs) EncodeFields(w model.FieldWriter) {
	w.String("msg", m.Msg)
}
func (m *mockArgs) DecodeFields(d model.FieldReader) {
	if val, ok := d.String("msg"); ok {
		m.Msg = val
	}
}
func (m *mockArgs) IsNil() bool { return m == nil }
func (m *mockArgs) Schema() []model.Field { return nil }
func (m *mockArgs) Pointers() []any       { return nil }

type mockModule struct{}

func (m mockModule) ModelName() string { return "testmod" }
func (m mockModule) MountOperations(reg router.OperationRegistry) {
	reg.Operation("echo", func(c router.Context) {
		var args mockArgs
		if err := c.Decode(&args); err != nil {
			c.WriteStatus(400)
			return
		}
		c.Encode(&args)
	}).Public().Accepts((*mockArgs)(nil)).Describe("Echoes the args")

	reg.Operation("secret", func(c router.Context) {
		c.Write([]byte("secret stuff"))
	}).Requires("thing", model.Read)

	reg.Operation("conflict", func(c router.Context) {
		c.WriteStatus(409)
		c.Write([]byte("taken"))
	}).Public()
}

func setupTestServer(t *testing.T) (*httptest.Server, router.Router) {
	mux := http.NewServeMux()
	r := httpd.NewRouter(mux)
	if err := rpc.Mount(r, rpc.DefaultPrefix, mockModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	ts := httptest.NewServer(mux)
	return ts, r
}

func TestRPCEcho(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	args := &mockArgs{Msg: "hello world"}
	var out mockArgs

	doneCh := make(chan error, 1)
	caller.Call("testmod.echo", args, &out, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}

	if out.Msg != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", out.Msg)
	}
}

func TestRPCRoutes(t *testing.T) {
	_, r := setupTestServer(t)

	routes := r.Routes()

	var foundEcho, foundSecret bool
	for _, route := range routes {
		if route.Method == "POST" && route.Path == "/api/testmod/echo" {
			foundEcho = true
			if route.Access != model.AccessPublic {
				t.Errorf("echo route should be public, got %v", route.Access)
			}
			if route.Description != "Echoes the args" {
				t.Errorf("expected desc 'Echoes the args', got %s", route.Description)
			}
		}
		if route.Method == "POST" && route.Path == "/api/testmod/secret" {
			foundSecret = true
			if route.Resource != "thing" || route.Action != model.Read {
				t.Errorf("secret route requires thing read, got %s %v", route.Resource, route.Action)
			}
		}
	}

	if !foundEcho {
		t.Error("echo route not found")
	}
	if !foundSecret {
		t.Error("secret route not found")
	}
}

func TestRPCSecretUnauthorized(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	args := &mockArgs{Msg: "hello world"}

	doneCh := make(chan error, 1)
	caller.Call("testmod.secret", args, nil, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		e, ok := err.(*rpc.Error)
		if !ok {
			t.Fatalf("expected *rpc.Error, got %T", err)
		}
		if e.Status < 400 || e.Status > 499 {
			t.Fatalf("expected 4xx status, got %d", e.Status)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCCSRF(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/testmod/echo", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 415 {
		t.Fatalf("expected 415, got %d", resp.StatusCode)
	}
}

func TestRPCConflict(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)

	doneCh := make(chan error, 1)
	caller.Call("testmod.conflict", nil, nil, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		e, ok := err.(*rpc.Error)
		if !ok {
			t.Fatalf("expected *rpc.Error, got %T", err)
		}
		if e.Status != 409 {
			t.Fatalf("expected 409 status, got %d", e.Status)
		}
		if e.Body != "taken" {
			t.Fatalf("expected 'taken' body, got '%s'", e.Body)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCUnreachable(t *testing.T) {
	ts, _ := setupTestServer(t)
	ts.Close() // intentionally close to trigger network error

	caller := rpc.NewCaller(ts.URL)

	doneCh := make(chan error, 1)
	caller.Call("testmod.echo", nil, nil, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		e, ok := err.(*rpc.Error)
		if !ok {
			t.Fatalf("expected *rpc.Error, got %T", err)
		}
		if e.Status != 0 {
			t.Fatalf("expected Status 0, got %d", e.Status)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCMountErrors(t *testing.T) {
	r := httpd.NewRouter(http.NewServeMux())

	err := rpc.Mount(r, "api", mockModule{})
	if err == nil {
		t.Fatal("expected error for bad prefix 'api'")
	}

	err = rpc.Mount(r, "/api/", mockModule{})
	if err == nil {
		t.Fatal("expected error for bad prefix '/api/'")
	}

	err = rpc.Mount(r, rpc.DefaultPrefix, emptyModule{})
	if err == nil {
		t.Fatal("expected error for empty ModelName")
	}

	err = rpc.Mount(r, rpc.DefaultPrefix, mockModule{}, mockModule{})
	if err == nil {
		t.Fatal("expected error for duplicate op path")
	}
}

type emptyModule struct{}
func (emptyModule) ModelName() string { return "" }
func (emptyModule) MountOperations(reg router.OperationRegistry) {}

type dupModule struct{}
func (dupModule) ModelName() string { return "testmod" }
func (dupModule) MountOperations(reg router.OperationRegistry) {
	reg.Operation("echo", func(c router.Context) {})
}

func TestRPCBadOp(t *testing.T) {
	caller := rpc.NewCaller("")

	doneCh := make(chan error, 1)
	caller.Call("noDot", nil, nil, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		e, ok := err.(*rpc.Error)
		if !ok {
			t.Fatalf("expected *rpc.Error, got %T", err)
		}
		if e.Status != 0 {
			t.Fatalf("expected 0 status, got %d", e.Status)
		}
		if e.Body != "rpc: operation name must be <module>.<op>" {
			t.Fatalf("expected bad op message, got %s", e.Body)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}
