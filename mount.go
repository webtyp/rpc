package rpc

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

// DefaultPrefix is where operations are mounted: POST /api/<module>/<op>.
const DefaultPrefix = "/api"

const (
	slash                      = "/"
	dot                        = "."
	contentTypeJSON            = "application/json"
	contentTypeName            = "Content-Type"
	statusUnsupportedMediaType = 415
	msgNeedJSON                = "rpc: Content-Type must be application/json"
)

const (
	errEmptyModelName = rpcError("rpc: module with empty ModelName()")
	errBadPrefix      = rpcError("rpc: prefix must start with \"/\" and not end with \"/\"")
)

func errDuplicatePath(path string) error {
	return rpcError(fmt.Sprintf("rpc: duplicate operation path %s", path))
}

type collectingRegistry struct {
	paths  []string
	prefix string
	module string
}

type dummyRoute struct{}

func (d dummyRoute) Requires(resource model.Resource, action model.Action) router.Route { return d }
func (d dummyRoute) Authenticated() router.Route                                        { return d }
func (d dummyRoute) Public() router.Route                                               { return d }
func (d dummyRoute) Accepts(schema model.Fielder) router.Route                          { return d }
func (d dummyRoute) Describe(desc string) router.Route                                  { return d }

func (r *collectingRegistry) Operation(name string, h router.HandlerFunc) router.Route {
	path := r.prefix + slash + r.module + slash + name
	r.paths = append(r.paths, path)
	return dummyRoute{}
}

type activeRegistry struct {
	r      router.Router
	prefix string
	module string
}

func (r *activeRegistry) Operation(name string, h router.HandlerFunc) router.Route {
	path := r.prefix + slash + r.module + slash + name
	return r.r.Post(path, guard(h))
}

func guard(h router.HandlerFunc) router.HandlerFunc {
	return func(c router.Context) {
		ct := c.GetHeader(contentTypeName)
		if len(ct) < len(contentTypeJSON) || ct[:len(contentTypeJSON)] != contentTypeJSON {
			c.WriteStatus(statusUnsupportedMediaType)
			c.Write([]byte(msgNeedJSON))
			return
		}
		h(c)
	}
}

// Mount registers every operation of every module as POST <prefix>/<ModelName>/<op> on r, with
// the access, arguments and description the module declared on the returned Route.
// It returns an error, and mounts nothing, when a module has an empty ModelName(), when prefix
// does not start with "/" or ends with "/", or when two operations resolve to the same path.
func Mount(r router.Router, prefix string, modules ...router.OperationModule) error {
	if len(prefix) == 0 || prefix[0] != '/' || prefix[len(prefix)-1] == '/' {
		return errBadPrefix
	}

	col := &collectingRegistry{
		prefix: prefix,
	}

	for _, m := range modules {
		modName := m.ModelName()
		if modName == "" {
			return errEmptyModelName
		}
		col.module = modName
		m.MountOperations(col)
	}

	for i := 0; i < len(col.paths); i++ {
		for j := i + 1; j < len(col.paths); j++ {
			if col.paths[i] == col.paths[j] {
				return errDuplicatePath(col.paths[i])
			}
		}
	}

	act := &activeRegistry{
		r:      r,
		prefix: prefix,
	}
	for _, m := range modules {
		act.module = m.ModelName()
		m.MountOperations(act)
	}

	return nil
}
