package rpc

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

// ErrPublicNotMounted is what JSON.MountOperations returns when a crud.Public names an action no module offers.
var ErrPublicNotMounted = fmt.Err("rpc: a Public action is not offered by any module — remove it from the app's Public list or add the module that has it")

// JSON publishes the operations as JSON over HTTP, at <DefaultPrefix>/<module>/<entity>. The app names it
// where it chooses its transports; appkit only sees the small interface it declares.
//
//	appkit.Backend{API: rpc.JSON{}}
type JSON struct{}

// MountOperations mounts every operation of ops on r. public lists the actions visitors may call without a session;
// a public action no module offers is an error and nothing stays half mounted. log is called on a 500; nil = silent.
func (JSON) MountOperations(r router.Router, ops []crud.Module, public []crud.Public, pub events.Publisher, log func(...any)) error {
	if len(ops) > 0 {
		if err := Mount(r, DefaultPrefix, log, public, pub, ops...); err != nil {
			return err
		}
	}
	return checkPublic(public, ops)
}

func checkPublic(public []crud.Public, ops []crud.Module) error {
	if len(public) == 0 {
		return nil
	}
	routes, err := Routes(DefaultPrefix, public, ops...)
	if err != nil {
		return err
	}
	actions := []model.Action{model.Create, model.Read, model.Update, model.Delete}
	for _, p := range public {
		suffix := "/" + p.Model.ModelName()
		for _, act := range actions {
			if !p.Actions.Has(act) {
				continue
			}
			method := MethodOf(act)
			found := false
			for _, ri := range routes {
				if ri.Access == model.AccessPublic && ri.Method == method && fmt.HasSuffix(ri.Path, suffix) {
					found = true
					break
				}
			}
			if !found {
				return fmt.ErrType(fmt.Err(p.Model.ModelName()), ErrPublicNotMounted)
			}
		}
	}
	return nil
}
