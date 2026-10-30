package rpc_test

import (
	"strings"
	"testing"

	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/rpc"
)

type saveArgs struct{}

func (s *saveArgs) EncodeFields(w model.FieldWriter) {}
func (s *saveArgs) DecodeFields(d model.FieldReader) {}
func (s *saveArgs) IsNil() bool                      { return s == nil }
func (s *saveArgs) Schema() []model.Field            { return nil }
func (s *saveArgs) Pointers() []any                  { return nil }

type routesModule struct{ name string }

func (m routesModule) ModelName() string { return m.name }
func (m routesModule) MountOperations(reg router.OperationRegistry) {
	noop := func(router.Context) {}
	reg.Operation("list", noop).Requires("demo_item", model.Read)
	reg.Operation("me", noop).Authenticated()
	reg.Operation("health", noop).Public()
	reg.Operation("save", noop).Requires("demo_item", model.Create).RequiresMFA().Accepts(&saveArgs{}).Describe("save items")
}

func TestRoutes_Declarations(t *testing.T) {
	routes, err := rpc.Routes(rpc.DefaultPrefix, routesModule{"demo"})
	if err != nil {
		t.Fatalf("Routes failed: %v", err)
	}

	if len(routes) != 4 {
		t.Fatalf("expected 4 routes, got %d", len(routes))
	}

	r0 := routes[0]
	if r0.Method != "POST" || r0.Path != "/api/demo/list" || r0.Resource != "demo_item" || r0.Action != model.Read || r0.Access != 0 || r0.MFA || r0.Args != nil || r0.Description != "" {
		t.Errorf("route 0 mismatch: %+v", r0)
	}

	r1 := routes[1]
	if r1.Method != "POST" || r1.Path != "/api/demo/me" || r1.Resource != "" || r1.Action != 0 || r1.Access != model.AccessAuthenticated || r1.MFA || r1.Args != nil || r1.Description != "" {
		t.Errorf("route 1 mismatch: %+v", r1)
	}

	r2 := routes[2]
	if r2.Method != "POST" || r2.Path != "/api/demo/health" || r2.Resource != "" || r2.Action != 0 || r2.Access != model.AccessPublic || r2.MFA || r2.Args != nil || r2.Description != "" {
		t.Errorf("route 2 mismatch: %+v", r2)
	}

	r3 := routes[3]
	if r3.Method != "POST" || r3.Path != "/api/demo/save" || r3.Resource != "demo_item" || r3.Action != model.Create || r3.Access != 0 || !r3.MFA || r3.Args == nil || r3.Description != "save items" {
		t.Errorf("route 3 mismatch: %+v", r3)
	}
}

func TestRoutes_EmptyModelName(t *testing.T) {
	routes, err := rpc.Routes(rpc.DefaultPrefix, routesModule{""})
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
		routes, err := rpc.Routes(p, routesModule{"demo"})
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
	routes, err := rpc.Routes(rpc.DefaultPrefix, routesModule{"demo"}, routesModule{"demo"})
	if err == nil {
		t.Fatal("expected error for duplicate, got nil")
	}
	if !strings.HasPrefix(err.Error(), "rpc: duplicate operation path") {
		t.Errorf("expected error to start with 'rpc: duplicate operation path', got '%v'", err)
	}
	if !strings.Contains(err.Error(), "/api/demo/list") {
		t.Errorf("expected error to contain '/api/demo/list', got '%v'", err)
	}
	if routes != nil {
		t.Errorf("expected nil routes on duplicate, got %v", routes)
	}
}

func TestMount_SameValidation(t *testing.T) {
	r := &routerMock{}
	err := rpc.Mount(r, rpc.DefaultPrefix, nil, routesModule{"demo"}, routesModule{"demo"})
	if err == nil {
		t.Fatal("expected error for Mount with duplicate, got nil")
	}
	if !strings.HasPrefix(err.Error(), "rpc: duplicate operation path") {
		t.Errorf("expected error to start with 'rpc: duplicate operation path', got '%v'", err)
	}
	if !strings.Contains(err.Error(), "/api/demo/list") {
		t.Errorf("expected error to contain '/api/demo/list', got '%v'", err)
	}
}

type routerMock struct{
	router.Router
}

func (m *routerMock) Post(path string, h router.HandlerFunc) router.Route {
	return nil
}
