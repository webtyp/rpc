package rpc_test

import (
	"errors"
	"testing"

	"webtyp.com/crud"
	"webtyp.com/model"
	"webtyp.com/router/mock"
	"webtyp.com/rpc"
)

type strayRec struct{ publicRec }

func (*strayRec) ModelName() string { return "stray" }

func TestJSON_MountsAPublicActionTheModuleOffers(t *testing.T) {
	public := []crud.Public{{Model: &publicRec{}, Actions: model.Read}}
	if err := (rpc.JSON{}).MountOperations(&mock.Router{}, []crud.Module{publicTestMod{}}, public, nil, nil); err != nil {
		t.Fatalf("a public action the module offers must mount, got %v", err)
	}
}

func TestJSON_PublicActionNoModuleOffersIsAnError(t *testing.T) {
	public := []crud.Public{{Model: &strayRec{}, Actions: model.Read}}
	err := (rpc.JSON{}).MountOperations(&mock.Router{}, []crud.Module{publicTestMod{}}, public, nil, nil)
	if !errors.Is(err, rpc.ErrPublicNotMounted) {
		t.Fatalf("expected ErrPublicNotMounted, got %v", err)
	}
}

func TestJSON_NothingToMountIsNotAnError(t *testing.T) {
	if err := (rpc.JSON{}).MountOperations(&mock.Router{}, nil, nil, nil, nil); err != nil {
		t.Fatalf("no operations and nothing public mounts nothing, got %v", err)
	}
}
