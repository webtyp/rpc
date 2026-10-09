---
PLAN: "feat: rpc — every router.OperationModule as POST /api/<module>/<op>, and a browser router.Caller for it"
TAG: v0.1.0
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 5244204706492890482
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `rpc`: domain operations over plain HTTP

## 0. Context (read first)

This repository is new and empty (created with `gonew`; module `webtyp.com/rpc`). It is one
piece of the "API surface" wave (the master plan lives in the maintainer's workspace; everything
you need is restated here).

In the webtyp ecosystem a **domain module** declares its operations once, against a
transport-neutral registry, and a **view** calls them through a transport-neutral caller. Both
contracts live in `webtyp.com/router` (v0.3.4):

```go
// mount side
type OperationRegistry interface {
    Operation(name string, h HandlerFunc) Route
}
type OperationModule interface {
    model.ModuleNaming          // ModelName() string
    MountOperations(reg OperationRegistry)
}
// Route (returned by Operation, Get, Post, ...): Requires(resource, action), Authenticated(),
// Public(), Accepts(model.Fielder), Describe(string) — each returns Route.

// call side
type Caller interface {
    // op is the QUALIFIED name "<ModelName>.<op>", e.g. "patient_directory.upsert_patient".
    Call(op string, args model.Encodable, into model.Decodable, done func(err error))
    Dispatch(op string, args model.Encodable) // fire-and-forget
}

// HTTP surface: Router.Post(path string, h HandlerFunc) Route; Router.Routes() []RouteInfo
// HandlerFunc func(Context); Context has Decode(model.Decodable) error, Encode(model.Encodable)
// error, WriteStatus(int), Write([]byte), GetHeader(string) string, UserID() string.
```

Today the only transports for operations are `webtyp.com/mcp` (`mcp.HarvestOps` turns every
operation into an MCP tool behind one `POST /mcp`) and `webtyp.com/router/loopback` (in process).
Using MCP as the browser↔server channel exposes **every** operation (115 in the clinic app) to
any agent, and hides all of them from the route table (`/_routes` shows a single `POST /mcp`).

This library is the HTTP binding: each operation becomes its own route, with the permission,
arguments and description the module declared, so it is visible in `Router.Routes()`, served by
any `router.Router` implementation, and callable from the browser.

## Design gate

### 1. Prior art
- **Connect RPC** (Buf): `POST /<package>.<Service>/<Method>`, JSON body, status codes mapped
  from errors, no REST verb mapping. Our shape.
- **Twirp** (Twitch): `POST /twirp/<package>.<Service>/<Method>` with JSON; deliberately not
  REST, one URL per procedure.
- **tRPC**: `/trpc/<router>.<procedure>`; a typed client generated from the server's procedures.
  Same idea as our `Caller`, without code generation because the args are already typed models.
- **REST (Rails resources, JAX-RS)**: rejected. Our operations are named commands
  (`change_reservation_status`, `overbook`), and mapping them onto CRUD verbs loses meaning and
  duplicates decisions the module already made.

### 2. Novice-name test
`rpc.Mount(r, rpc.DefaultPrefix, modules...)` — "mount these modules' operations under /api".
`rpc.NewCaller("")` — "a caller that talks to this server". `rpc.Error{Status, Body}` — "the
server answered with this status".

### 3. Complexity ledger
```
Concepts the developer must learn   +2 (Mount, NewCaller) / −0
Files they must touch to do X        0 (the composition root swaps one line)
Lines at the call site               +1 server / ±0 client
Ways to do the same thing            0 now; −1 when apps stop calling ops through mcp.NewCaller
```

### 4. Where it belongs
One concern, "operations over HTTP", with both ends in one repository — the same pattern as
`webtyp/mcp` and `webtyp/sse`. Not in `router`: the client needs `webtyp/fetch` and
`webtyp/json`, and `router` is the contract package every module depends on.

### 5. What this deletes
Nothing in this repository (new capability). It enables deleting `mcp.NewCaller` from browser
apps (done in each app's integration plan, not here).

## 1. Target API (exactly this is exported)

```go
package rpc

// DefaultPrefix is where operations are mounted: POST /api/<module>/<op>.
const DefaultPrefix = "/api"

// Mount registers every operation of every module as POST <prefix>/<ModelName>/<op> on r, with
// the access, arguments and description the module declared on the returned Route.
// It returns an error, and mounts nothing, when a module has an empty ModelName(), when prefix
// does not start with "/" or ends with "/", or when two operations resolve to the same path.
func Mount(r router.Router, prefix string, modules ...router.OperationModule) error

// NewCaller returns a router.Caller that sends each Call as POST <origin><prefix>/<module>/<op>
// with a JSON body. origin "" means the page's own origin (relative URLs). Uses DefaultPrefix.
func NewCaller(origin string) router.Caller

// Error is what Call reports when the server answers outside 2xx, or when the request never
// got an answer (Status 0). Callers that must tell "refused" (4xx) from "try later" (0, 5xx)
// assert it: if e, ok := err.(*rpc.Error); ok { ... }.
type Error struct {
    Status int    // HTTP status; 0 = network failure, no answer
    Body   string // response body as text (the handler's message), or the network error text
}
func (e *Error) Error() string // "rpc: <status> <body>"; for Status 0: "rpc: no response: <body>"
```

## 2. Behaviour (normative)

### 2.1 Mount (server side; no build tag — it only uses `router`)
- An unexported `registry` implements `router.OperationRegistry`. For each module it sets the
  current module name and calls `m.MountOperations(reg)`.
- `reg.Operation(name, h)` returns `r.Post(prefix+"/"+module+"/"+name, guard(h))`. Returning the
  real `router.Route` means the module's `.Requires/.Authenticated/.Public/.Accepts/.Describe`
  land on the HTTP route itself: the router's own RBAC gate enforces them and `Routes()` reports
  them. No copy of the access rules lives in this package.
- **Validate before mounting**: run every module's `MountOperations` against a *collecting*
  registry first (records the paths, returns a no-op `router.Route`), check the errors of §1,
  then mount for real. Error texts are unexported typed constants
  (`type rpcError string` + `Error()`), e.g. `rpc: module with empty ModelName()`,
  `rpc: prefix must start with "/" and not end with "/"`, `rpc: duplicate operation path <path>`.
- `guard(h)` wraps the handler: if the request's `Content-Type` (via `ctx.GetHeader`) does not
  start with `application/json`, answer `415` with body `rpc: Content-Type must be application/json`
  and do not call `h`. **Why:** the native server also decodes HTML form bodies; a cross-site
  `<form method=post>` cannot send `application/json` without a CORS preflight, so this closes
  CSRF for cookie-authenticated operations. Document this in the README.

### 2.2 NewCaller (client side; works in WASM and stdlib through `webtyp/fetch`)
- `Call(op, args, into, done)`:
  - `op` must contain exactly one `.`; otherwise `done(&Error{Status: 0, Body: "rpc: operation name must be <module>.<op>"})`.
  - URL = `origin + DefaultPrefix + "/" + module + "/" + opName`.
  - Body = `json.Encode(args)` (`webtyp.com/json`); `args == nil` → `{}`.
  - `fetch.Post(url).ContentTypeJSON().Body(body).Send(...)` — mirror how
    `webtyp.com/mcp`'s `client.go` builds its request (read it in the module cache).
  - fetch error → `done(&Error{Status: 0, Body: err.Error()})`.
  - `resp.Status` outside 200–299 → `done(&Error{Status: resp.Status, Body: resp.Text()})`.
  - 2xx: `into == nil` or empty body → `done(nil)`; else `done(json.Decode(resp.Body(), into))`.
  - `done == nil` is allowed (no callback).
- `Dispatch(op, args)` = the same request with no callback.

## 3. Stages

| Stage | Files | Content |
|---|---|---|
| 1 | `go.mod` | `go get webtyp.com/router webtyp.com/model webtyp.com/json webtyp.com/fetch webtyp.com/fmt`; delete gonew's placeholder `rpc.go` content |
| 2 | `mount.go`, `errors.go` | §2.1 |
| 3 | `caller.go` | §2.2, `Error` |
| 4 | `tests/` | §4 |
| 5 | `README.md`, `docs/ARCHITECTURE.md` | "I want X → use Y" table, the URL shape, the 415/CSRF rule, the example of §5 |

## 4. Tests (`tests/`, external package `rpc_test`; run with `gotest`)

Consumer-shaped: a real `webtyp.com/server/httpd` server (`httptest`-style, as other webtyp
repos test httpd — read `server/httpd` tests for how to build one and get its handler), a small
test `OperationModule` declared in the test package (two ops: `echo` with `.Public().Accepts(...)`
that decodes args and encodes them back, and `secret` with `.Requires("thing", model.Read)`), and
the **real** `rpc.NewCaller` pointed at the test server's URL (stdlib lane of `webtyp/fetch`).

1. `Call("testmod.echo", args, &out, done)` → `out` equals `args`; the route is `POST /api/testmod/echo`.
2. `Routes()` of the router contains `POST /api/testmod/echo` (public) and `POST /api/testmod/secret`
   with resource `thing`, action read, and the `Describe` text.
3. `secret` without an identity → `*rpc.Error` with `Status` 401 or 403 (whatever the router's
   gate answers; assert it is 4xx and that the handler did not run).
4. A raw POST to `/api/testmod/echo` with `Content-Type: application/x-www-form-urlencoded` → 415
   and the handler did not run.
5. Handler writes `409` + `"taken"` → `*rpc.Error{Status: 409, Body: "taken"}`.
6. Server unreachable (closed server URL) → `*rpc.Error` with `Status: 0`.
7. `Mount` errors: empty `ModelName()`, prefix `"api"` and `"/api/"`, a module registering the same
   op twice → error, and **no** route was mounted.
8. `Call("noDot", ...)` → `*rpc.Error{Status: 0}` with the name message.

## 5. README example

```go
// server (composition root)
if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, booking, patients, records); err != nil {
    return err
}
// browser (composition root)
caller := rpc.NewCaller("")
view, _ := patientsui.Browser(caller, ids, tenantID)
```

## 6. Code rules (non-negotiable)
- Compiles to TinyGo WASM: `webtyp.com/fmt` for errors/formatting; no `errors`, `strconv`,
  `strings`, `encoding/json`, `net/http` in library code (tests may use stdlib).
- No `map` in library code (TinyGo binary size): slices and linear scans.
- Repeated strings (`"/"`, `"."`, `"application/json"`, header names, messages) are constants.
- No exported symbol beyond §1. Tests only in `tests/`; never export a symbol for a test.

## 7. Acceptance criteria
- `gotest ./...` green (stdlib and wasm lanes).
- `grep -rn "map\[" --include=*.go . | grep -v _test.go` → empty.
- `grep -rn "TODO\|FIXME" --include=*.go .` → empty.
