package rpc_test

import (
	"strings"
	"testing"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/rpc"
)

type routeItem struct {
	ID string
}

func (r *routeItem) Allow() model.Action { return model.Create | model.Read }
func (r *routeItem) ModelName() string   { return "route_item" }
func (r *routeItem) IsNil() bool         { return r == nil }
func (r *routeItem) Schema() []model.Field {
	return []model.Field{{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}}}
}
func (r *routeItem) Pointers() []any                  { return []any{&r.ID} }
func (r *routeItem) EncodeFields(w model.FieldWriter) { w.String("id", r.ID) }
func (r *routeItem) DecodeFields(d model.FieldReader) { r.ID, _ = d.String("id") }

type routeItemList []*routeItem

func (s *routeItemList) Len() int                         { return len(*s) }
func (s *routeItemList) At(i int) model.Fielder           { return (*s)[i] }
func (s *routeItemList) Append() model.Fielder            { v := &routeItem{}; *s = append(*s, v); return v }
func (s *routeItemList) IsNil() bool                      { return s == nil }
func (s *routeItemList) EncodeFields(_ model.FieldWriter) {}
func (s *routeItemList) DecodeFields(_ model.FieldReader) {}

type routesModule struct{ name string }

func (m routesModule) ModelName() string { return m.name }
func (m routesModule) MountOperations(reg crud.Registry) {
	reg.Table(crud.Table{
		Model:   &routeItem{},
		NewList: func() model.ModelSlice { return &routeItemList{} },
		Store:   fakeStore{},
		MFA:     model.Create,
	})
}

func TestRoutes_Declarations(t *testing.T) {
	routes, err := rpc.Routes(rpc.DefaultPrefix, nil, routesModule{"demo"})
	if err != nil {
		t.Fatalf("Routes failed: %v", err)
	}

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	var rCreate, rRead *router.RouteInfo
	for i := range routes {
		if routes[i].Action == model.Create {
			rCreate = &routes[i]
		} else if routes[i].Action == model.Read {
			rRead = &routes[i]
		}
	}
	if rCreate == nil || rRead == nil {
		t.Fatalf("expected create and read routes, got %+v", routes)
	}
	if rCreate.Method != "POST" || rCreate.Path != "/api/demo/route_item_c" || rCreate.Resource != "route_item" || !rCreate.MFA || rCreate.Description != "Create route_item" {
		t.Errorf("create route mismatch: %+v", rCreate)
	}
	if rRead.Method != "POST" || rRead.Path != "/api/demo/route_item_r" || rRead.Resource != "route_item" || rRead.MFA || rRead.Description != "Read route_item" {
		t.Errorf("read route mismatch: %+v", rRead)
	}
}

func TestRoutes_EmptyModelName(t *testing.T) {
	routes, err := rpc.Routes(rpc.DefaultPrefix, nil, routesModule{""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "rpc: module with empty ModelName()" {
		t.Errorf("expected error 'rpc: module with empty ModelName()', got '%v'", err)
	}
	if routes != nil {
		t.Errorf("expected nil routes, got %v", routes)
	}
}

func TestRoutes_BadPrefix(t *testing.T) {
	badPrefixes := []string{"api", "/api/"}
	for _, p := range badPrefixes {
		routes, err := rpc.Routes(p, nil, routesModule{"demo"})
		if err == nil {
			t.Errorf("expected error for prefix %q", p)
		}
		if err.Error() != "rpc: prefix must start with \"/\" and not end with \"/\"" {
			t.Errorf("expected bad prefix error, got %v", err)
		}
		if routes != nil {
			t.Errorf("expected nil routes for prefix %q", p)
		}
	}
}

func TestRoutes_Duplicate(t *testing.T) {
	_, err := rpc.Routes(rpc.DefaultPrefix, nil, routesModule{"demo"}, routesModule{"demo"})
	if err == nil {
		t.Fatal("expected error for duplicate, got nil")
	}
	if !strings.HasPrefix(err.Error(), "rpc: duplicate operation path") {
		t.Errorf("expected error to start with 'rpc: duplicate operation path', got '%v'", err)
	}
	if !strings.Contains(err.Error(), "/api/demo/route_item_c") {
		t.Errorf("expected error to contain '/api/demo/route_item_c', got '%v'", err)
	}
}
