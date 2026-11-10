package rpc_test

import (
	"testing"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router/mock"
	"webtyp.com/rpc"
)

type publicRec struct {
	ID string
}

func (r *publicRec) Allow() model.Action { return model.Create | model.Read }
func (r *publicRec) ModelName() string   { return "rec" }
func (r *publicRec) IsNil() bool         { return r == nil }
func (r *publicRec) Schema() []model.Field {
	return []model.Field{{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}}}
}
func (r *publicRec) Pointers() []any                  { return []any{&r.ID} }
func (r *publicRec) EncodeFields(w model.FieldWriter) { w.String("id", r.ID) }
func (r *publicRec) DecodeFields(d model.FieldReader) { r.ID, _ = d.String("id") }

type publicRecList []*publicRec

func (s *publicRecList) Len() int                         { return len(*s) }
func (s *publicRecList) At(i int) model.Fielder           { return (*s)[i] }
func (s *publicRecList) Append() model.Fielder            { v := &publicRec{}; *s = append(*s, v); return v }
func (s *publicRecList) IsNil() bool                      { return s == nil }
func (s *publicRecList) EncodeFields(_ model.FieldWriter) {}
func (s *publicRecList) DecodeFields(_ model.FieldReader) {}

type publicTestMod struct{}

func (publicTestMod) ModelName() string { return "pubmod" }
func (publicTestMod) Definition() model.ModuleDefinition {
	return defOf("pubmod", model.Definition{
		Name:  "rec",
		Allow: model.Create | model.Read,
		Rules: model.Rules{
			Create: model.HaveNoBusinessRule,
			Read:   model.HaveNoBusinessRule,
		},
	})
}
func (publicTestMod) MountOperations(reg crud.Registry) {
	reg.AddStoredEntity(crud.StoredEntity{
		Model:   &publicRec{},
		NewList: func() model.ModelSlice { return &publicRecList{} },
		Store:   fakeStore{},
		Rules: model.Rules{
			Create: model.HaveNoBusinessRule,
			Read:   model.HaveNoBusinessRule,
		},
	})
}

func TestRoutes_PublicCreate(t *testing.T) {
	routes, err := rpc.Routes("/api", []crud.Public{{Model: &publicRec{}, Actions: model.Create}}, publicTestMod{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}

	var foundRead, foundCreate bool
	for _, r := range routes {
		if r.Path == "/api/pubmod/rec" && r.Method == "POST" {
			foundCreate = true
			if r.Access != model.AccessPublic {
				t.Errorf("expected rec POST to have AccessPublic, got %v", r.Access)
			}
		}
		if r.Path == "/api/pubmod/rec" && r.Method == "GET" {
			foundRead = true
			if r.Access == model.AccessPublic {
				t.Errorf("expected rec GET to NOT have AccessPublic")
			}
		}
	}
	if !foundCreate {
		t.Errorf("rec POST route not found")
	}
	if !foundRead {
		t.Errorf("rec GET route not found")
	}
}

func TestRoutes_PublicReadStillWorks(t *testing.T) {
	routes, err := rpc.Routes("/api", []crud.Public{{Model: &publicRec{}, Actions: model.Read}}, publicTestMod{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}

	var foundRead, foundCreate bool
	for _, r := range routes {
		if r.Path == "/api/pubmod/rec" && r.Method == "GET" {
			foundRead = true
			if r.Access != model.AccessPublic {
				t.Errorf("expected rec GET to have AccessPublic, got %v", r.Access)
			}
		}
		if r.Path == "/api/pubmod/rec" && r.Method == "POST" {
			foundCreate = true
			if r.Access == model.AccessPublic {
				t.Errorf("expected rec POST to NOT have AccessPublic")
			}
		}
	}
	if !foundRead {
		t.Errorf("rec GET route not found")
	}
	if !foundCreate {
		t.Errorf("rec POST route not found")
	}
}

func TestRoutes_NoPublicByDefault(t *testing.T) {
	routes, err := rpc.Routes("/api", nil, publicTestMod{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}

	for _, r := range routes {
		if r.Path == "/api/pubmod/rec" && r.Method == "GET" {
			if r.Access == model.AccessPublic {
				t.Errorf("rec GET should not be public by default")
			}
			if r.Resource != "rec" || r.Action != model.Read {
				t.Errorf("expected rec to have Resource=rec, Action=Read; got %v %v", r.Resource, r.Action)
			}
		}
	}
}

type errWiringMod struct{}

func (errWiringMod) ModelName() string { return "errmod" }
func (errWiringMod) Definition() model.ModuleDefinition {
	return defOf("errmod", model.Definition{
		Name:  "errrec",
		Allow: model.Create,
		Rules: model.Rules{Create: model.HaveNoBusinessRule},
	})
}
func (errWiringMod) MountOperations(reg crud.Registry) {
	reg.AddStoredEntity(crud.StoredEntity{})
}

func TestMount_CrudWiringErrorStopsMount(t *testing.T) {
	r := &mock.Router{}
	err := rpc.Mount(r, "/api", nil, nil, nil, errWiringMod{})
	if err != crud.ErrNilFunction {
		t.Fatalf("expected crud.ErrNilFunction, got %v", err)
	}
}
