package rpc_test

import (
	"webtyp.com/model"
)

// defOf is the Definition of a fixture module that mounts one Table (or one wire Read): its name and the entity's Allow and Rules.
func defOf(module string, handlers ...model.Definition) model.ModuleDefinition {
	return model.ModuleDefinition{Name: module, Entities: handlers}
}
