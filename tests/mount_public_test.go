package rpc_test

import (
	"testing"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/rpc"
	"webtyp.com/server/httpd"
	"net/http"
)

type publicRec struct {
	ID string
}

func (r *publicRec) ModelName() string { return "rec" }
func (r *publicRec) IsNil() bool       { return r == nil }
func (r *publicRec) Schema() []model.Field {
	return []model.Field{{Name: "id", Type: model.Text()}}
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
func (publicTestMod) MountOperations(reg crud.Registry) {
	reg.Read(&publicRec{}, nil, func(c crud.Caller, args crud.ReadArgs, filter model.Model) (model.ModelSlice, error) {
		out := publicRecList{}
		return &out, nil
	})
	reg.Create(func() model.ModelSlice { return &publicRecList{} }, func(c crud.Caller, list model.ModelSlice) error {
		return nil
	})
}

func TestRoutes_PublicReadFromApp(t *testing.T) {
	routes, err := rpc.Routes("/api", []model.ModuleNaming{&publicRec{}}, publicTestMod{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}

	var foundRead, foundCreate bool
	for _, r := range routes {
		if r.Path == "/api/pubmod/rec_r" {
			foundRead = true
			if r.Access != model.AccessPublic {
				t.Errorf("expected rec_r to have AccessPublic, got %v", r.Access)
			}
		}
		if r.Path == "/api/pubmod/rec_c" {
			foundCreate = true
			if r.Resource != "rec" || r.Action != model.Create {
				t.Errorf("expected rec_c to have Resource=rec, Action=Create; got %v %v", r.Resource, r.Action)
			}
		}
	}
	if !foundRead {
		t.Errorf("rec_r route not found")
	}
	if !foundCreate {
		t.Errorf("rec_c route not found")
	}
}

func TestRoutes_NoPublicByDefault(t *testing.T) {
	routes, err := rpc.Routes("/api", nil, publicTestMod{})
	if err != nil {
		t.Fatalf("unexpected Routes error: %v", err)
	}

	for _, r := range routes {
		if r.Path == "/api/pubmod/rec_r" {
			if r.Access == model.AccessPublic {
				t.Errorf("rec_r should not be public by default")
			}
			if r.Resource != "rec" || r.Action != model.Read {
				t.Errorf("expected rec_r to have Resource=rec, Action=Read; got %v %v", r.Resource, r.Action)
			}
		}
	}
}

type errWiringMod struct{}

func (errWiringMod) ModelName() string { return "errmod" }
func (errWiringMod) MountOperations(reg crud.Registry) {
	reg.Delete(&publicRec{}, nil)
}

func TestMount_CrudWiringErrorStopsMount(t *testing.T) {
	r := httpd.NewRouter(http.NewServeMux())
	err := rpc.Mount(r, "/api", nil, nil, errWiringMod{})
	if err != crud.ErrNilFunction {
		t.Fatalf("expected crud.ErrNilFunction, got %v", err)
	}
}
