---
PLAN: "feat: rpc.Routes — the routes of every operation, with its access, without a server"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 13550848540781111546
---

> Read AGENTS.md at the repo root before any change.

# `rpc.Routes`

## Context

A module's demo runs in the browser with no server, and its API explorer must show the routes its operations would
have on a server: `POST /api/<module>/<op>` with the permission, public/authenticated flag, MFA, arguments and
description each operation declared. `rpc.Mount` already walks every operation with a `collectingRegistry`, but it
only keeps the **paths** (to detect duplicates) and its `dummyRoute` throws the declarations away (`mount.go:30-46`).
This plan makes that walk keep everything, as `rpc.Routes(prefix, modules...)`, and `Mount` validates through it —
one walk, used by the server and by the demo.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`. Tests are in the separate module
`tests/` (see AGENTS.md); `gotest` from the repo root runs them.

## Ecosystem search

Searched `collectingRegistry`, `dummyRoute`, `Routes(` over `webtyp/`: the operation walk exists only here; the server
gets its table from the real router after `Mount`. **Extend** `rpc`; `dummyRoute` is **deleted**.

## Design gate

1. **Prior art.** gRPC server reflection lists services and methods from the registered handlers without calling them;
   OpenAPI generators (e.g. FastAPI) build the route list from the declarations, not from traffic (unverified: pages
   not read in this session). Same: the list comes from the declarations.
2. **Novice name.** `rpc.Routes(rpc.DefaultPrefix, mods...)` — "the routes these modules get".
3. **Complexity ledger.** Concepts +0 (`router.RouteInfo` exists) · walks of the operations: one (shared by `Mount`).
4. **Where it belongs.** `rpc`, owner of the `POST <prefix>/<module>/<op>` convention.
5. **Deletes.** `dummyRoute` and `collectingRegistry.paths`.

## APIs used

| Symbol | Exact signature | Tag |
|---|---|---|
| `router.RouteInfo` | fields `Method, Path string; Resource model.Resource; Action model.Action; Access model.Access; MFA bool; Description string; Args model.Fielder` (zero `Access` is `model.AccessGuarded`) | `webtyp.com/router` v0.4.16 |
| `router.Route` | interface: `Requires(model.Resource, model.Action) Route; Authenticated() Route; Public() Route; RequiresMFA() Route; Accepts(model.Fielder) Route; Describe(string) Route` | v0.4.16 |
| `router.OperationModule` | `ModelName() string; MountOperations(reg OperationRegistry)` | v0.4.16 |
| `model.AccessAuthenticated`, `model.AccessPublic` | constants of `model.Access` | `webtyp.com/model` |

If a signature differs, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1 — tests first: `tests/routes_test.go` (new, `package rpc_test`)

Use a module declared in this file:

```go
type routesModule struct{ name string }

func (m routesModule) ModelName() string { return m.name }
func (m routesModule) MountOperations(reg router.OperationRegistry) {
	noop := func(router.Context) {}
	reg.Operation("list", noop).Requires("demo_item", model.Read)
	reg.Operation("me", noop).Authenticated()
	reg.Operation("health", noop).Public()
	reg.Operation("save", noop).Requires("demo_item", model.Create).RequiresMFA().Accepts(&saveArgs{}).Describe("save items")
}
```

(`saveArgs`: a minimal `model.Fielder` with one field, like `dummyArgs` in the router tests.) Red after this stage.

| Test | Call | Expected |
|---|---|---|
| `TestRoutes_Declarations` | `rpc.Routes(rpc.DefaultPrefix, routesModule{"demo"})` | 4 routes, in declaration order, all `Method "POST"`: `/api/demo/list` guarded `demo_item` read; `/api/demo/me` `AccessAuthenticated`; `/api/demo/health` `AccessPublic`; `/api/demo/save` guarded `demo_item` create, `MFA`, `Args != nil`, `Description "save items"` |
| `TestRoutes_EmptyModelName` | module with `""` | error text `"rpc: module with empty ModelName()"`, `nil` routes |
| `TestRoutes_BadPrefix` | prefix `"api"` and `"/api/"` | error text of `errBadPrefix` (`rpc: prefix must start with "/" and not end with "/"`) |
| `TestRoutes_Duplicate` | two `routesModule{"demo"}` | error starting `"rpc: duplicate operation path"` and containing `/api/demo/list` |
| `TestMount_SameValidation` | `rpc.Mount` with the duplicate case | the same error as `Routes` |

Existing tests in `tests/rpc_test.go` must keep passing unchanged. Commit after this stage.

## Stage 2 — `mount.go`

Replace `collectingRegistry` and `dummyRoute` with:

```go
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
```

In `Mount`, replace everything before `act := &activeRegistry{` with:

```go
	if _, err := Routes(prefix, modules...); err != nil {
		return err
	}
```

Run `gotest`: all green. Commit after this stage.

## Stage 3 — docs

`README.md`: a short section — `rpc.Routes(prefix, modules...)` lists the routes `Mount` would create, with their
access, without a server (used by a module's demo for its API explorer). Commit.

## Delivery check

1. `gotest` passes, `tests/rpc_test.go` unchanged. A PR that does not compile, or whose tests could not run, is a failed delivery.
2. `grep -n "dummyRoute\|paths " mount.go` → empty.
3. `git diff --stat main` lists only `tests/routes_test.go`, `mount.go`, `README.md`.

## Delivery rules

- Never pause to ask permission or report progress: when the delivery check passes, commit, push and open the PR. Stuck
  for 30 minutes: open the PR anyway, listing what fails.
- Commit after every stage. Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `tests/routes_test.go` | Declarations kept; three failures; Mount validates the same; red |
| 2 | `mount.go` | One walk; `dummyRoute` deleted; green |
| 3 | `README.md` | Documented |
