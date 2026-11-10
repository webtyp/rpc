package rpc

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

// DefaultPrefix is where operations are mounted: <METHOD> /api/<module>/<entity>.
const DefaultPrefix = "/api"

const (
	slash                      = "/"
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

func errInvalidOperation(op router.Operation) error {
	return fmt.Err("rpc: invalid operation", op.String())
}

// collectingRegistry walks the operations and keeps the route each one gets.
type collectingRegistry struct {
	routes []router.RouteInfo
	prefix string
	err    error
}

func (r *collectingRegistry) Operation(op router.Operation, h router.HandlerFunc) router.Route {
	method, path, ok := requestOf(r.prefix, op)
	if !ok {
		if r.err == nil {
			r.err = errInvalidOperation(op)
		}
		return inertRoute{}
	}
	r.routes = append(r.routes, router.RouteInfo{Method: method, Path: path})
	return collectingRoute{reg: r, i: len(r.routes) - 1}
}

// inertRoute swallows the declarations of an operation that was refused; the registry reports the error.
type inertRoute struct{}

func (r inertRoute) Requires(model.Resource, model.Action) router.Route { return r }
func (r inertRoute) Authenticated() router.Route                        { return r }
func (r inertRoute) Public() router.Route                               { return r }
func (r inertRoute) RequiresMFA() router.Route                          { return r }
func (r inertRoute) Accepts(model.Fielder) router.Route                 { return r }
func (r inertRoute) Describe(string) router.Route                       { return r }

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
// (<METHOD> <prefix>/<module>/<entity>), with the access, arguments and description
// it declared — without a server. Public actions come from the app (`crud.Public`), never from a module.
//
// Example: routes, err := rpc.Routes(rpc.DefaultPrefix, public, accessMod)
func Routes(prefix string, public []crud.Public, modules ...crud.Module) ([]router.RouteInfo, error) {
	if len(prefix) == 0 || prefix[0] != '/' || prefix[len(prefix)-1] == '/' {
		return nil, errBadPrefix
	}
	col := &collectingRegistry{prefix: prefix}
	for _, m := range modules {
		if m.ModelName() == "" {
			return nil, errEmptyModelName
		}
		if err := crud.Mount(col, public, nil, m); err != nil {
			return nil, err
		}
		if col.err != nil {
			return nil, col.err
		}
	}
	for i := 0; i < len(col.routes); i++ {
		for j := i + 1; j < len(col.routes); j++ {
			if col.routes[i].Method == col.routes[j].Method && col.routes[i].Path == col.routes[j].Path {
				return nil, errDuplicatePath(col.routes[i].Method + " " + col.routes[i].Path)
			}
		}
	}
	return col.routes, nil
}

type activeRegistry struct {
	r      router.Router
	prefix string
	log    func(...any)
	err    error
}

func (r *activeRegistry) Operation(op router.Operation, h router.HandlerFunc) router.Route {
	method, path, ok := requestOf(r.prefix, op)
	if !ok {
		if r.err == nil {
			r.err = errInvalidOperation(op)
		}
		return inertRoute{}
	}
	g := guard(method, h, path, r.log)
	switch method {
	case methodGet:
		return r.r.Get(path, g)
	case methodPost:
		return r.r.Post(path, g)
	case methodDelete:
		return r.r.Delete(path, g)
	}
	return r.r.Handle(method, path, g) // PATCH: router.Router has no Patch
}

func guard(method string, h router.HandlerFunc, path string, log func(...any)) router.HandlerFunc {
	return func(c router.Context) {
		c.SetHeader(headerCacheControl, cacheNoStore)
		if hasBody(method) {
			ct := c.GetHeader(contentTypeName)
			if len(ct) < len(contentTypeJSON) || ct[:len(contentTypeJSON)] != contentTypeJSON {
				c.WriteStatus(statusUnsupportedMediaType)
				c.Write([]byte(msgNeedJSON))
				return
			}
		}
		h(c)
		if d := router.InternalError(c); d != "" && log != nil {
			log("rpc:", path, "internal error:", d)
		}
	}
}

// Mount registers every operation of every module as <METHOD> <prefix>/<module>/<entity> on r, with
// the access, arguments and description the module declared on the returned Route.
// Public actions come from the app (`crud.Public`), never from a module.
// It returns an error, and mounts nothing, when a module has an empty ModelName(), when prefix
// does not start with "/" or ends with "/", or when two operations resolve to the same path.
// log is an optional logger called when an operation returns a 500 internal error.
func Mount(r router.Router, prefix string, log func(...any), public []crud.Public, pub events.Publisher, modules ...crud.Module) error {
	if _, err := Routes(prefix, public, modules...); err != nil {
		return err
	}
	act := &activeRegistry{r: r, prefix: prefix, log: log}
	for _, m := range modules {
		if err := crud.Mount(act, public, pub, m); err != nil {
			return err
		}
		if act.err != nil {
			return act.err
		}
	}
	return nil
}
