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
	"webtyp.com/storage"
)

type mockRecord struct {
	ID  string
	Msg string
}

func (m *mockRecord) Allow() model.Action { return model.AllActions }
func (m *mockRecord) ModelName() string   { return "mock_rec" }
func (m *mockRecord) IsNil() bool         { return m == nil }
func (m *mockRecord) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "msg", Type: model.Text(), Item: model.ItemLabel},
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

type fakeStore struct{}

func (fakeStore) ReadOne(rec model.Model, id string) error { return nil }
func (fakeStore) ReadAll(newRow func() model.Model, q crud.Query, onRow func(model.Model)) error {
	m := newRow()
	*m.Pointers()[0].(*string) = "1"
	if len(q.Conditions) > 0 {
		if s, ok := q.Conditions[0].Value().(string); ok {
			*m.Pointers()[1].(*string) = s
		}
	}
	onRow(m)
	return nil
}
func (fakeStore) Create(rec model.Model) error { return nil }
func (fakeStore) UpdateFields(rec model.Model, id string, fields []string) error {
	return nil
}
func (fakeStore) Delete(rec model.Model, id string) error { return nil }
func (fakeStore) NewID() string                           { return "1" }

type mockModule struct{}

var (
	keyHits atomic.Int32
	lastKey atomic.Value
)

func (m mockModule) ModelName() string { return "testmod" }
func (m mockModule) MountOperations(reg crud.Registry) {
	reg.Table(crud.Table{
		Model:   &mockRecord{},
		NewList: func() model.ModelSlice { return &mockRecordList{} },
		Store:   fakeStore{},
		Search: func(c crud.Caller, text string) ([]storage.Condition, error) {
			return []storage.Condition{storage.Eq("msg", text)}, nil
		},
		Update: func(c crud.Caller, tx crud.Tx, old, next model.Model, fields []string) error {
			keyHits.Add(1)
			return nil
		},
		Delete: func(c crud.Caller, tx crud.Tx, rec model.Model) error {
			return model.Conflict("taken")
		},
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
	if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, nil, []crud.Public{{Model: &mockRecord{}, Actions: model.Read}}, mockModule{}); err != nil {
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
			if route.Description != "Read mock_rec" {
				t.Errorf("expected desc 'Read mock_rec', got %s", route.Description)
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
