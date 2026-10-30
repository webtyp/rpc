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

var (
	errEmptyModelName error = fmt.Err("rpc: module with empty ModelName()")
	errBadPrefix      error = fmt.Err("rpc: prefix must start with \"/\" and not end with \"/\"")
)

func errDuplicatePath(path string) error {
	return fmt.Err("rpc: duplicate operation path", path)
}

const methodPost = "POST"

// collectingRegistry walks the operations and keeps the route each one gets.
type collectingRegistry struct {
	routes []router.RouteInfo
	prefix string
	module string
}

func (r *collectingRegistry) Operation(name string, h router.HandlerFunc) router.Route {
	r.routes = append(r.routes, router.RouteInfo{Method: methodPost, Path: r.prefix + slash + r.module + slash + name})
	return collectingRoute{reg: r, i: len(r.routes) - 1}
}

// collectingRoute records what an operation declares on its route.
type collectingRoute struct {
	reg *collectingRegistry
	i   int
}

func (c collectingRoute) Requires(resource model.Resource, action model.Action) router.Route {
	c.reg.routes[c.i].Resource = resource
	c.reg.routes[c.i].Action = action
	return c
}
func (c collectingRoute) Authenticated() router.Route {
	c.reg.routes[c.i].Access = model.AccessAuthenticated
	return c
}
func (c collectingRoute) Public() router.Route {
	c.reg.routes[c.i].Access = model.AccessPublic
	return c
}
func (c collectingRoute) RequiresMFA() router.Route {
	c.reg.routes[c.i].MFA = true
	return c
}
func (c collectingRoute) Accepts(schema model.Fielder) router.Route {
	c.reg.routes[c.i].Args = schema
	return c
}
func (c collectingRoute) Describe(desc string) router.Route {
	c.reg.routes[c.i].Description = desc
	return c
}

// Routes returns the route every operation of every module gets under prefix
// (POST <prefix>/<ModelName>/<op>), with the access, arguments and description
// it declared — without a server. Mount validates through it.
//
// Example: routes, err := rpc.Routes(rpc.DefaultPrefix, accessMod)
func Routes(prefix string, modules ...router.OperationModule) ([]router.RouteInfo, error) {
	if len(prefix) == 0 || prefix[0] != '/' || prefix[len(prefix)-1] == '/' {
		return nil, errBadPrefix
	}
	col := &collectingRegistry{prefix: prefix}
	for _, m := range modules {
		modName := m.ModelName()
		if modName == "" {
			return nil, errEmptyModelName
		}
		col.module = modName
		m.MountOperations(col)
	}
	for i := 0; i < len(col.routes); i++ {
		for j := i + 1; j < len(col.routes); j++ {
			if col.routes[i].Path == col.routes[j].Path {
				return nil, errDuplicatePath(col.routes[i].Path)
			}
		}
	}
	return col.routes, nil
}

type activeRegistry struct {
	r      router.Router
	prefix string
	module string
	log    func(...any)
}

func (r *activeRegistry) Operation(name string, h router.HandlerFunc) router.Route {
	path := r.prefix + slash + r.module + slash + name
	return r.r.Post(path, guard(h, path, r.log))
}

func guard(h router.HandlerFunc, path string, log func(...any)) router.HandlerFunc {
	return func(c router.Context) {
		ct := c.GetHeader(contentTypeName)
		if len(ct) < len(contentTypeJSON) || ct[:len(contentTypeJSON)] != contentTypeJSON {
			c.WriteStatus(statusUnsupportedMediaType)
			c.Write([]byte(msgNeedJSON))
			return
		}
		h(c)
		if d := c.Value(ctxKeyInternal); d != "" && log != nil {
			log("rpc:", path, "internal error:", d)
		}
	}
}

// Mount registers every operation of every module as POST <prefix>/<ModelName>/<op> on r, with
// the access, arguments and description the module declared on the returned Route.
// It returns an error, and mounts nothing, when a module has an empty ModelName(), when prefix
// does not start with "/" or ends with "/", or when two operations resolve to the same path.
// log is an optional logger called when an operation returns a 500 internal error.
func Mount(r router.Router, prefix string, log func(...any), modules ...router.OperationModule) error {
	if _, err := Routes(prefix, modules...); err != nil {
		return err
	}

	act := &activeRegistry{
		r:      r,
		prefix: prefix,
		log:    log,
	}
	for _, m := range modules {
		act.module = m.ModelName()
		m.MountOperations(act)
	}

	return nil
}
