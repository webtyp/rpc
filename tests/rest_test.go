package rpc_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/router/mock"
	"webtyp.com/router/routertest"
	"webtyp.com/rpc"
)

func TestMethodOf_PerAction(t *testing.T) {
	if got := rpc.MethodOf(model.Create); got != "POST" {
		t.Errorf("expected POST, got %s", got)
	}
	if got := rpc.MethodOf(model.Read); got != "GET" {
		t.Errorf("expected GET, got %s", got)
	}
	if got := rpc.MethodOf(model.Update); got != "PATCH" {
		t.Errorf("expected PATCH, got %s", got)
	}
	if got := rpc.MethodOf(model.Delete); got != "DELETE" {
		t.Errorf("expected DELETE, got %s", got)
	}
	if got := rpc.MethodOf(model.Create | model.Read); got != "" {
		t.Errorf("expected empty string for composite action, got %s", got)
	}
	if got := rpc.MethodOf(0); got != "" {
		t.Errorf("expected empty string for 0, got %s", got)
	}
}

type demoItem struct {
	ID   string
	Name string
}

func (d *demoItem) Allow() model.Action { return model.AllActions }
func (d *demoItem) ModelName() string   { return "item" }
func (d *demoItem) IsNil() bool         { return d == nil }
func (d *demoItem) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text()},
	}
}
func (d *demoItem) Pointers() []any                  { return []any{&d.ID, &d.Name} }
func (d *demoItem) EncodeFields(w model.FieldWriter) { w.String("id", d.ID); w.String("name", d.Name) }
func (d *demoItem) DecodeFields(r model.FieldReader) {
	d.ID, _ = r.String("id")
	d.Name, _ = r.String("name")
}

type demoItemList []*demoItem

func (s *demoItemList) Len() int                         { return len(*s) }
func (s *demoItemList) At(i int) model.Fielder           { return (*s)[i] }
func (s *demoItemList) Append() model.Fielder            { v := &demoItem{}; *s = append(*s, v); return v }
func (s *demoItemList) IsNil() bool                      { return s == nil }
func (s *demoItemList) EncodeFields(_ model.FieldWriter) {}
func (s *demoItemList) DecodeFields(_ model.FieldReader) {}

type demoAllModule struct{}

func (demoAllModule) ModelName() string { return "demo" }
func (demoAllModule) Definition() model.ModuleDefinition {
	return defOf("demo", model.Definition{
		Name:  "item",
		Allow: model.AllActions,
		Rules: model.Rules{
			Create: model.HaveNoBusinessRule,
			Read:   model.HaveNoBusinessRule,
			Update: model.HaveNoBusinessRule,
			Delete: model.HaveNoBusinessRule,
		},
	})
}
func (demoAllModule) MountOperations(reg crud.Registry) {
	reg.AddStoredEntity(crud.StoredEntity{
		Model:   &demoItem{},
		NewList: func() model.ModelSlice { return &demoItemList{} },
		Store:   fakeStore{},
		Rules: model.Rules{
			Create: model.HaveNoBusinessRule,
			Read:   model.HaveNoBusinessRule,
			Update: model.HaveNoBusinessRule,
			Delete: model.HaveNoBusinessRule,
		},
	})
}

func TestRequest_PerAction(t *testing.T) {
	routes, err := rpc.Routes("/api", nil, demoAllModule{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}
	if len(routes) != 4 {
		t.Fatalf("expected 4 routes, got %d", len(routes))
	}

	want := map[string]model.Action{
		"POST":   model.Create,
		"GET":    model.Read,
		"PATCH":  model.Update,
		"DELETE": model.Delete,
	}
	for _, r := range routes {
		if r.Path != "/api/demo/item" {
			t.Errorf("expected path /api/demo/item, got %s", r.Path)
		}
		expectedAction, ok := want[r.Method]
		if !ok {
			t.Errorf("unexpected method: %s", r.Method)
		} else if r.Action != expectedAction {
			t.Errorf("method %s: expected action %v, got %v", r.Method, expectedAction, r.Action)
		}
	}
}

type readWireModel struct{}

func (readWireModel) ModelName() string { return "read_entity" }

type readTestMod struct {
	gotArgs crud.ReadArgs
}

func (m *readTestMod) ModelName() string { return "testread" }
func (m *readTestMod) Definition() model.ModuleDefinition {
	return defOf("testread", model.Definition{
		Name:  "read_entity",
		Wire:  true,
		Allow: model.Read,
		Rules: model.Rules{Read: model.AddMyBusinessRule},
	})
}
func (m *readTestMod) MountOperations(reg crud.Registry) {
	reg.AddReadOnlyEntity(readWireModel{}, nil, func(c crud.CurrentUser, args crud.ReadArgs, filter model.Model) (model.ModelSlice, error) {
		m.gotArgs = args
		return &mockRecordList{}, nil
	})
}

func TestRPCGet_ArgumentsTravelInTheQuery(t *testing.T) {
	var rawBody []byte
	var rawCT string

	r := &mock.Router{}
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) {
			rawBody = c.Body()
			rawCT = c.GetHeader("Content-Type")
			next(c)
		}
	})

	mod := &readTestMod{}
	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, []crud.Public{{Model: readWireModel{}, Actions: model.Read}}, nil, mod); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	ts := routertest.Serve(t, r)
	caller := rpc.NewCaller(ts.URL)
	readOp := router.Operation{Module: "testread", Entity: "read_entity", Action: model.Read}

	var out mockRecordList
	doneCh := make(chan error, 1)
	caller.Call(readOp, &crud.ReadArgs{Search: "12.345", Limit: 20}, &out, func(err error) {
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

	if mod.gotArgs.Search != "12.345" {
		t.Errorf("expected search '12.345', got %q", mod.gotArgs.Search)
	}
	if mod.gotArgs.Limit != 20 {
		t.Errorf("expected limit 20, got %d", mod.gotArgs.Limit)
	}
	if len(rawBody) != 0 {
		t.Errorf("expected empty raw request body, got %q", string(rawBody))
	}
	if rawCT != "" {
		t.Errorf("expected no Content-Type header on GET, got %q", rawCT)
	}
}

type typedFilter struct {
	Name string
}

func (f *typedFilter) ModelName() string   { return "typed_filter" }
func (f *typedFilter) IsNil() bool         { return f == nil }
func (f *typedFilter) Schema() []model.Field {
	return []model.Field{{Name: "name", Type: model.Text()}}
}
func (f *typedFilter) Pointers() []any                  { return []any{&f.Name} }
func (f *typedFilter) EncodeFields(w model.FieldWriter) { w.String("name", f.Name) }
func (f *typedFilter) DecodeFields(r model.FieldReader) { f.Name, _ = r.String("name") }

type filterTestMod struct {
	gotFilter model.Model
}

func (m *filterTestMod) ModelName() string { return "testfilter" }
func (m *filterTestMod) Definition() model.ModuleDefinition {
	return defOf("testfilter", model.Definition{
		Name:  "read_entity",
		Wire:  true,
		Allow: model.Read,
		Rules: model.Rules{Read: model.AddMyBusinessRule},
	})
}
func (m *filterTestMod) MountOperations(reg crud.Registry) {
	reg.AddReadOnlyEntity(readWireModel{}, func() model.Model { return &typedFilter{} }, func(c crud.CurrentUser, args crud.ReadArgs, filter model.Model) (model.ModelSlice, error) {
		m.gotFilter = filter
		return &mockRecordList{}, nil
	})
}

func TestRPCGet_FilterTravelsAsPrefixedKeys(t *testing.T) {
	var rawPath string

	r := &mock.Router{}
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) {
			rawPath = c.Path()
			next(c)
		}
	})

	mod := &filterTestMod{}
	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, []crud.Public{{Model: readWireModel{}, Actions: model.Read}}, nil, mod); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	ts := routertest.Serve(t, r)
	caller := rpc.NewCaller(ts.URL)
	readOp := router.Operation{Module: "testfilter", Entity: "read_entity", Action: model.Read}

	var out mockRecordList
	doneCh := make(chan error, 1)
	caller.Call(readOp, &crud.ReadArgs{Filter: &typedFilter{Name: "x"}}, &out, func(err error) {
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

	if !strings.Contains(rawPath, "filter.name=x") {
		t.Errorf("expected request URL to contain 'filter.name=x', got %q", rawPath)
	}
	if mod.gotFilter == nil {
		t.Fatal("expected non-nil filter in callback")
	}
	tf, ok := mod.gotFilter.(*typedFilter)
	if !ok || tf.Name != "x" {
		t.Errorf("expected filter.name=='x', got %+v", mod.gotFilter)
	}
}

func setupRestServer(t *testing.T) (*httptest.Server, router.Router) {
	r := &mock.Router{}
	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, []crud.Public{
		{Model: &demoItem{}, Actions: model.AllActions},
	}, nil, demoAllModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}
	ts := routertest.Serve(t, r)
	return ts, r
}

func TestRPCResponses_AreNoStore(t *testing.T) {
	ts, _ := setupRestServer(t)
	defer ts.Close()

	getResp, err := http.Get(ts.URL + "/api/demo/item")
	if err != nil {
		t.Fatal(err)
	}
	getResp.Body.Close()
	if cc := getResp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("GET: expected Cache-Control: no-store, got %q", cc)
	}

	postResp, err := http.Post(ts.URL+"/api/demo/item", "application/json", strings.NewReader(`{"records":[{"name":"test"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	postResp.Body.Close()
	if cc := postResp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("POST: expected Cache-Control: no-store, got %q", cc)
	}
}

func TestRPCMutations_RequireJSON(t *testing.T) {
	ts, _ := setupRestServer(t)
	defer ts.Close()

	mutations := []string{"POST", "PATCH", "DELETE"}
	for _, m := range mutations {
		req, err := http.NewRequest(m, ts.URL+"/api/demo/item", strings.NewReader("text"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "text/plain")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 415 {
			t.Errorf("method %s: expected 415 for text/plain, got %d", m, resp.StatusCode)
		}
	}

	getReq, err := http.NewRequest("GET", ts.URL+"/api/demo/item", nil)
	if err != nil {
		t.Fatal(err)
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Errorf("GET: expected 200 without Content-Type, got %d", getResp.StatusCode)
	}
}

func TestRPCUpdate_UsesPatchWithBody(t *testing.T) {
	var gotMethod string
	var gotBody string

	r := &mock.Router{}
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) {
			gotMethod = c.Method()
			gotBody = string(c.Body())
			next(c)
		}
	})

	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, []crud.Public{{Model: &demoItem{}, Actions: model.Update}}, nil, demoAllModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	ts := routertest.Serve(t, r)
	caller := rpc.NewCaller(ts.URL)
	updateOp := router.Operation{Module: "demo", Entity: "item", Action: model.Update}

	doneCh := make(chan error, 1)
	caller.Call(updateOp, &crud.UpdateArgs{IDs: []string{"a"}, Fields: []string{"name"}, Record: &demoItem{Name: "val"}}, nil, func(err error) {
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

	if gotMethod != "PATCH" {
		t.Errorf("expected PATCH, got %s", gotMethod)
	}
	if !strings.Contains(gotBody, `"ids":["a"]`) && !strings.Contains(gotBody, `"a"`) {
		t.Errorf("expected body to contain id 'a', got %q", gotBody)
	}
}

func TestRPCDelete_UsesDeleteWithBody(t *testing.T) {
	var gotMethod string
	var gotBody string

	r := &mock.Router{}
	r.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) {
			gotMethod = c.Method()
			gotBody = string(c.Body())
			next(c)
		}
	})

	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, []crud.Public{{Model: &demoItem{}, Actions: model.Delete}}, nil, demoAllModule{}); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	ts := routertest.Serve(t, r)
	caller := rpc.NewCaller(ts.URL)
	deleteOp := router.Operation{Module: "demo", Entity: "item", Action: model.Delete}

	doneCh := make(chan error, 1)
	caller.Call(deleteOp, &crud.DeleteArgs{IDs: []string{"a", "b"}}, nil, func(err error) {
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

	if gotMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(gotBody, `"ids":["a","b"]`) {
		t.Errorf("expected body to contain '{\"ids\":[\"a\",\"b\"]}', got %q", gotBody)
	}
}

func TestRPCCallKeyed_RefusesARead(t *testing.T) {
	caller := rpc.NewCaller("")
	readOp := router.Operation{Module: "testmod", Entity: "mock_rec", Action: model.Read}

	doneCh := make(chan error, 1)
	caller.CallKeyed(readOp, "k", nil, nil, func(err error) {
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
		if !strings.Contains(strings.ToLower(e.Body), "read") {
			t.Errorf("expected error message to contain 'read', got %q", e.Body)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRPCInvalidOperation(t *testing.T) {
	caller := rpc.NewCaller("")

	doneCh := make(chan error, 1)
	caller.Call(router.Operation{}, nil, nil, func(err error) {
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
		if !strings.Contains(strings.ToLower(e.Body), "operation") {
			t.Errorf("expected error message to contain 'operation', got %q", e.Body)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}
