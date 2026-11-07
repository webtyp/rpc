package rpc_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/rpc"
	"webtyp.com/server/httpd"
)

type mockRecord struct {
	ID  string
	Msg string
}

func (m *mockRecord) ModelName() string { return "mock_rec" }
func (m *mockRecord) IsNil() bool       { return m == nil }
func (m *mockRecord) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text()},
		{Name: "msg", Type: model.Text()},
	}
}
func (m *mockRecord) Pointers() []any                  { return []any{&m.ID, &m.Msg} }
func (m *mockRecord) EncodeFields(w model.FieldWriter) { w.String("id", m.ID); w.String("msg", m.Msg) }
func (m *mockRecord) DecodeFields(d model.FieldReader) {
	m.ID, _ = d.String("id")
	m.Msg, _ = d.String("msg")
}

type mockRecordList []*mockRecord

func (s *mockRecordList) Len() int                         { return len(*s) }
func (s *mockRecordList) At(i int) model.Fielder           { return (*s)[i] }
func (s *mockRecordList) Append() model.Fielder            { v := &mockRecord{}; *s = append(*s, v); return v }
func (s *mockRecordList) IsNil() bool                      { return s == nil }
func (s *mockRecordList) EncodeFields(_ model.FieldWriter) {}
func (s *mockRecordList) DecodeFields(_ model.FieldReader) {}

type mockModule struct{}

var (
	keyHits atomic.Int32
	lastKey atomic.Value
)

func (m mockModule) ModelName() string { return "testmod" }
func (m mockModule) MountOperations(reg crud.Registry) {
	reg.Create(func() model.ModelSlice { return &mockRecordList{} }, func(c crud.Caller, list model.ModelSlice) error {
		// Used by echo test
		return nil
	}).Describe("Creates records")

	reg.Read(&mockRecord{}, nil, func(c crud.Caller, args crud.ReadArgs, filter model.Model) (model.ModelSlice, error) {
		out := mockRecordList{
			&mockRecord{ID: "1", Msg: args.Search},
		}
		return &out, nil
	}).Describe("Reads records")

	reg.Update(func() model.Model { return &mockRecord{} }, func(c crud.Caller, ids, fields []string, rec model.Model) error {
		keyHits.Add(1)
		return nil
	})

	reg.Delete(&mockRecord{}, func(c crud.Caller, ids []string) error {
		return model.Conflict("taken")
	})
}

func setupTestServer(t *testing.T) (*httptest.Server, router.Router) {
	srv := httpd.New(httpd.Config{
		Authorize: func(userID string, resource model.Resource, action model.Action) bool {
			// mock_rec Create is forbidden for unauthorized test
			if resource == "mock_rec" && action == model.Create {
				return false
			}
			return true
		},
		Authn: func(next router.HandlerFunc) router.HandlerFunc {
			return func(c router.Context) {
				c.SetUserID("test-user")
				next(c)
			}
		},
	})
	if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, nil, []model.ModuleNaming{&mockRecord{}}, mockModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	handler, err := srv.Handler()
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}

	ts := httptest.NewServer(handler)
	return ts, srv.Router()
}

func TestRPCEcho(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	args := &crud.ReadArgs{Search: "hello world"}
	var out mockRecordList

	doneCh := make(chan error, 1)
	caller.Call("testmod."+crud.Op(&mockRecord{}, model.Read), args, &out, func(err error) {
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

	if len(out) != 1 || out[0].Msg != "hello world" {
		t.Fatalf("expected 1 record with 'hello world', got %+v", out)
	}
}

func TestRPCCallKeyed(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	body := []byte(`{"ids":["id1"],"fields":["msg"],"record":{"msg":"keyed"}}`)

	doneCh := make(chan error, 1)
	caller.CallKeyed("testmod."+crud.Op(&mockRecord{}, model.Update), "k1", body, nil, func(err error) {
		doneCh <- err
	})

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("CallKeyed failed: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCCallKeyedEmptyKey(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	before := keyHits.Load()

	doneCh := make(chan error, 1)
	caller.CallKeyed("testmod."+crud.Op(&mockRecord{}, model.Update), "", []byte(`{}`), nil, func(err error) {
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
			t.Fatalf("expected status 0, got %d", e.Status)
		}
		if e.Body != "rpc: idempotency key is required" {
			t.Fatalf("expected idempotency key required error, got '%s'", e.Body)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}

	if hits := keyHits.Load() - before; hits != 0 {
		t.Fatalf("handler ran %d times; an empty key must send nothing", hits)
	}
}

func TestRPCRoutes(t *testing.T) {
	_, r := setupTestServer(t)

	routes := r.Routes()

	var foundRead, foundCreate bool
	for _, route := range routes {
		if route.Method == "POST" && route.Path == "/api/testmod/mock_rec_r" {
			foundRead = true
			if route.Access != model.AccessPublic {
				t.Errorf("read route should be public, got %v", route.Access)
			}
			if route.Description != "Reads records" {
				t.Errorf("expected desc 'Reads records', got %s", route.Description)
			}
		}
		if route.Method == "POST" && route.Path == "/api/testmod/mock_rec_c" {
			foundCreate = true
			if route.Resource != "mock_rec" || route.Action != model.Create {
				t.Errorf("create route requires mock_rec Create, got %s %v", route.Resource, route.Action)
			}
		}
	}

	if !foundRead {
		t.Error("read route not found")
	}
	if !foundCreate {
		t.Error("create route not found")
	}
}

func TestRPCSecretUnauthorized(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	caller := rpc.NewCaller(ts.URL)
	args := &crud.CreateArgs{
		Records: []model.Model{&mockRecord{Msg: "hello world"}},
	}

	doneCh := make(chan error, 1)
	caller.Call("testmod."+crud.Op(&mockRecord{}, model.Create), args, nil, func(err error) {
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

	resp, err := http.Post(ts.URL+"/api/testmod/mock_rec_r", "application/x-www-form-urlencoded", nil)
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
	caller.Call("testmod."+crud.Op(&mockRecord{}, model.Delete), &crud.DeleteArgs{IDs: []string{"id-1"}}, nil, func(err error) {
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
		if e.Detail != "taken" {
			t.Fatalf("expected 'taken' detail, got '%s'", e.Detail)
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
	caller.Call("testmod."+crud.Op(&mockRecord{}, model.Read), nil, nil, func(err error) {
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

	err := rpc.Mount(r, "api", nil, nil, mockModule{})
	if err == nil {
		t.Fatal("expected error for bad prefix 'api'")
	}

	err = rpc.Mount(r, "/api/", nil, nil, mockModule{})
	if err == nil {
		t.Fatal("expected error for bad prefix '/api/'")
	}

	err = rpc.Mount(r, rpc.DefaultPrefix, nil, nil, emptyModule{})
	if err == nil {
		t.Fatal("expected error for empty ModelName")
	}

	err = rpc.Mount(r, rpc.DefaultPrefix, nil, nil, mockModule{}, mockModule{})
	if err == nil {
		t.Fatal("expected error for duplicate op path")
	}
}

type emptyModule struct{}

func (emptyModule) ModelName() string                     { return "" }
func (emptyModule) MountOperations(reg crud.Registry) {}

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
