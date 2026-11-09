---
PLAN: "refactor!: operations travel as REST (POST|GET|PATCH|DELETE /api/<module>/<entity>); the client and the server share one mapping from router.Operation"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 14760273422388366097
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit after every stage, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> **The last step is `git push` and `gh pr create`; your final message is the pull request link, never a question.** A session that ends with a question has delivered nothing.

## Goal

Today every operation is `POST /api/<module>/<entity>_c|_r|_u|_d`, the verb hidden in a suffix, and both `mount.go` and `caller.go` build or parse that text.
After this plan the HTTP method is the action and the path is the resource, from one function used by the server and the client:

| `router.Operation.Action` | method | path | arguments |
|---|---|---|---|
| `model.Create` | `POST` | `/api/<module>/<entity>` | JSON body `{records}` |
| `model.Read` | `GET` | `/api/<module>/<entity>` | the query (`id`, `search`, `after`, `limit`, `filter.<field>`) |
| `model.Update` | `PATCH` | `/api/<module>/<entity>` | JSON body `{ids, fields, record}` |
| `model.Delete` | `DELETE` | `/api/<module>/<entity>` | JSON body `{ids}` |

Every response carries `Cache-Control: no-store` (a clinic's records must not be cached by a proxy or the browser). Mutations still require `Content-Type: application/json`
(a form post from another site is refused with 415); a `GET` has no body, so it requires nothing.

Execution order: `model`, `router` (v0.4.21, with `routertest.Serve`), `crud` and `fetch` are published before this plan starts. No `server` is needed: the tests drive `routertest.Serve`.

## Ecosystem search

Searched `requestOf`, `methodOf`, `DefaultPrefix`, `fetch.Post`, `collectingRegistry` over `webtyp/*`: only `rpc` maps an operation to HTTP; `mcp` and `crud.Loopback` do not use HTTP. **Extend** `rpc` (no new library). The query is encoded
with `router.EncodeQuery` and decoded server side by `router.QueryReader` (the `router` plan); the verbs come from `fetch` (`Get`, `Post`, `Patch` — the `fetch` plan —, `Delete`).

## Design gate

1. **Prior art** (unverified in this session): Stripe and GitHub APIs use method + resource; AWS Lambda separates a synchronous call from an asynchronous one with a typed invocation type.
2. **Novice-name test**: `GET /api/patient_directory/patient?search=12.345` says what it does with no manual.
3. **Ledger**: parsing sites −2 (`Caller.send`'s dot scan, the `<module>/<op>` path building in `mount.go`); mapping functions +1 (`requestOf`); ways to address an operation 2 → 1.
4. **Where**: `rpc` is the HTTP transport of operations; it owns the mapping.
5. **Deletes**: `msgBadOpName` and the dot-splitting in `Caller.send`; `activeRegistry.module`, `collectingRegistry.module`; the `Post`-only registration; `TestRPCBadOp`.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
```

Dependencies (all published, no `replace`): `go get webtyp.com/model@v0.2.18 webtyp.com/router@v0.4.21 webtyp.com/crud@v0.0.8 webtyp.com/fetch@v0.1.30 webtyp.com/events@v0.1.8 && go mod tidy`. `rpc` depends on no server library, not even in its tests: the `tests/` module (it has its own `go.mod`) drops `webtyp.com/server` and uses `router` v0.4.21: its tests start the standard library's test server in front of a `router/mock` router with `routertest.Serve(t, mockRouter)` (a `net/http/httptest` server; the mock `Context` decodes a `GET` from its query like every transport). The end-to-end test against `httpd` belongs to `appkit`, which composes both.

## APIs used

| symbol | exact signature | file |
|---|---|---|
| `router.Operation` | `struct{ Module, Entity string; Action model.Action }`; `Verb() string`, `Valid() bool`, `String() string` | `router/op.go` (router plan) |
| `router.Caller` | `Call(op Operation, args model.Encodable, into model.Decodable, done func(err error))`; `Dispatch(op Operation, args model.Encodable)` | `router/caller.go` (router plan) |
| `router.OperationRegistry` | `Operation(op Operation, h HandlerFunc) Route` | `router/router.go` (router plan) |
| `router.EncodeQuery` | `func EncodeQuery(v model.Encodable) (string, error)` | `router/query_codec.go` (router plan) |
| `router.Router` | `Get/Post/Put/Delete(path string, h HandlerFunc) Route`, `Handle(method, path string, h HandlerFunc) Route` (there is no `Patch`: use `Handle("PATCH", …)`) | `router/router.go:132` |
| `crud.Mount` | `func Mount(reg router.OperationRegistry, public []Public, pub events.Publisher, m Module) error` | `crud/mount.go` (crud plan) |
| `fetch.Get/Post/Patch/Delete` | `func X(endpoint any) *Request`; `(*Request).ContentTypeJSON()`, `.Header(k, v string)`, `.Body([]byte)`, `.Send(cb func(*Response, error))` | `fetch/fetch.go` |
| `events.Publisher` | `Publish(e Event)` | `events/events.go:43` |

If a signature differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 0 — the tests stop using `httpd` (decision D44: a transport contract depends on no server)

`rpc/tests/go.mod` drops `webtyp.com/server`; `go get webtyp.com/router@v0.4.21` there (it adds `routertest.Serve` and makes the mock `Context` decode a `GET` from its query). Convert the three existing places that built an `httpd` server, in this order, `gotest` green after each:

| Where | Before | After |
|---|---|---|
| `tests/rpc_test.go:96`, `setupTestServer` | `srv := httpd.New(httpd.Config{Authorize: …, Authn: …})`, `rpc.Mount(srv.Router(), …)`, `handler, _ := srv.Handler()`, `ts := httptest.NewServer(handler)` | `r := &mock.Router{}`, `r.Configure(mock.Config{Authorize: …, Authn: …})` (the same two functions), `rpc.Mount(r, …)`, `ts := routertest.Serve(t, r)`; return `ts` and `r` |
| `tests/rpc_test.go:347` and `tests/mount_public_test.go:131` | `r := httpd.NewRouter(http.NewServeMux())` | `r := &mock.Router{}` |

Imports: `webtyp.com/router/mock` and `webtyp.com/router/routertest` instead of `webtyp.com/server/httpd` (and drop `net/http` / `httptest` where no longer used). Behaviour does not change: the same assertions pass. Commit: `test: rpc tests run on router/mock, not httpd`.

## Stage 1 — tests first (red)

Fixture modules in `tests/` (every type with `MountOperations(reg crud.Registry)`: `mockModule`, `emptyModule`, `routesModule`, `publicTestMod`, `errWiringMod` and any other `grep -n "MountOperations" tests/*.go` finds)
now implement `Definition()`. New helper in `tests/helpers_test.go`:

```go
// defOf is the Definition of a fixture module that mounts one Table (or one wire Read): its name and the entity's Allow and Rules.
func defOf(module string, handlers ...model.Definition) model.ModuleDefinition {
	return model.ModuleDefinition{Name: module, Entities: handlers}
}
```

and each fixture returns `defOf(<its ModelName()>, model.Definition{Name: <entity ModelName()>, Allow: <entity Allow()>, Rules: <HaveNoBusinessRule for every allowed action>})`; for a wire `Read`
fixture the handler is `{Name: …, Wire: true, Allow: model.Read, Rules: model.Rules{Read: model.AddMyBusinessRule}}`. `emptyModule` (empty `ModelName()`) returns `defOf("")` so `Check()` fails first.
Change every `Table{…}` the fixtures build to carry `Rules:` equal to the Definition's, and every hook to the `(crud.RuleAnswer, error)` shape (see the `crud` plan, stage 1.1).

Edit the existing tests:

| Test | Change |
|---|---|
| `TestRoutes_Declarations` | `rCreate`: `Method=="POST"`, `Path=="/api/demo/route_item"`; `rRead`: `Method=="GET"`, same path; the other expectations (Resource, MFA, Description) unchanged. Find routes by `Method`, not by the old `_c`/`_r` suffix |
| `TestRoutes_Duplicate` | two modules named `demo` → error; message from `errDuplicatePath` now names method and path |
| `TestRPCEcho`, `TestRPCSecretUnauthorized`, `TestRPCConflict`, `TestRPCUnreachable` | replace `"testmod."+crud.Op(&mockRecord{}, model.X)` by `router.Operation{Module: "testmod", Entity: "mock_rec", Action: model.X}` |
| `TestRPCRoutes` | the checks `route.Method == "POST" && route.Path == "/api/testmod/mock_rec_r"` become `route.Method == "GET" && route.Path == "/api/testmod/mock_rec"`, and `_c` → `POST` same path |
| `TestRPCCSRF` | posts `application/x-www-form-urlencoded` to `/api/testmod/mock_rec` with `POST`, then `PATCH` and `DELETE` (use `http.NewRequest`) → each answers 415 |
| `TestRPCBadOp` | **deleted** (no strings to get wrong) |

New tests (file `tests/rest_test.go`, using `routertest.Serve` like `rpc_test.go`):

| Test | setup → call → expected |
|---|---|
| `TestMethodOf_PerAction` | `MethodOf(Create)=="POST"`, `Read`→`"GET"`, `Update`→`"PATCH"`, `Delete`→`"DELETE"`, `Create\|Read` and `0` → `""` |
| `TestRequest_PerAction` | `Routes("/api", nil, mod)` for entity `item` of module `demo` → exactly four routes, path `/api/demo/item`, methods `POST`, `GET`, `PATCH`, `DELETE` for Create, Read, Update, Delete |
| `TestRPCGet_ArgumentsTravelInTheQuery` | `caller.Call(readOp, &crud.ReadArgs{Search: "12.345", Limit: 20}, &out, done)` → the server's read callback receives `args.Search=="12.345"`, `args.Limit==20`; the raw request has no body and no `Content-Type` |
| `TestRPCGet_FilterTravelsAsPrefixedKeys` | a read with a typed filter `{name: "x"}` → the request URL contains `filter.name=x`; the callback receives the filter decoded |
| `TestRPCResponses_AreNoStore` | `GET` and `POST` answers have header `Cache-Control: no-store` |
| `TestRPCMutations_RequireJSON` | `POST`, `PATCH`, `DELETE` with `Content-Type: text/plain` → 415; `GET` without any content type → 200 |
| `TestRPCUpdate_UsesPatchWithBody` | `Call(updateOp, &crud.UpdateArgs{IDs: ["a"], Fields: ["name"], Record: …})` → server sees method `PATCH` and the JSON body with those ids |
| `TestRPCDelete_UsesDeleteWithBody` | `Call(deleteOp, &crud.DeleteArgs{IDs: ["a","b"]})` → server sees method `DELETE` and body `{"ids":["a","b"]}` |
| `TestRPCCallKeyed_RefusesARead` | `CallKeyed(readOp, "k", nil, nil, done)` → `done` receives `*rpc.Error` with `Status 0` and text containing `read` |
| `TestRPCInvalidOperation` | `Call(router.Operation{}, nil, nil, done)` → `done` receives `*rpc.Error` with `Status 0` and text containing `operation`; nothing is sent |

Run `gotest`: it does not compile. Commit: `test: REST operations (red)`.

## Stage 2 — the mapping (`request.go`, new)

```go
package rpc

import (
	"webtyp.com/model"
	"webtyp.com/router"
)

const (
	methodGet    = "GET"
	methodPost   = "POST"
	methodPatch  = "PATCH"
	methodDelete = "DELETE"

	headerCacheControl = "Cache-Control"
	cacheNoStore       = "no-store"
)

// MethodOf is the HTTP method an action uses: POST to create, GET to read, PATCH to update, DELETE to delete; "" for anything else.
// appkit uses it to find the route of a public action; the server and the client use it through requestOf.
//
//	rpc.MethodOf(model.Read) // "GET"
func MethodOf(a model.Action) string {
	switch a {
	case model.Create:
		return methodPost
	case model.Read:
		return methodGet
	case model.Update:
		return methodPatch
	case model.Delete:
		return methodDelete
	}
	return ""
}

// requestOf is the HTTP request an operation uses: the method is the action, the path is the resource.
// The server registers it and the client sends it, so the two cannot drift apart. An invalid operation has none.
//
//	method, path, ok := requestOf("/api", router.Operation{Module: "agenda", Entity: "patient", Action: model.Read}) // GET /api/agenda/patient
func requestOf(prefix string, op router.Operation) (method, path string, ok bool) {
	if !op.Valid() {
		return "", "", false
	}
	return MethodOf(op.Action), prefix + slash + op.Module + slash + op.Entity, true
}

// hasBody reports whether requests of this method carry a JSON body.
func hasBody(method string) bool { return method != methodGet }
```

Add to `mount.go` constants: `errInvalidOperation` (see stage 3). `gotest` still red until stage 3. No commit alone.

## Stage 3 — the server side (`mount.go`)

Delete `const methodPost`, `collectingRegistry.module`, `activeRegistry.module` and the `module` lines in `Routes`/`Mount`. Replace the two registries and `guard`, `Routes` and `Mount`:

```go
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
```

`Routes`: call `crud.Mount(col, public, nil, m)` (no publisher: it only lists routes), return `col.err` if set after the loop, and compare **method and path** in the duplicate check:

```go
			if col.routes[i].Method == col.routes[j].Method && col.routes[i].Path == col.routes[j].Path {
				return nil, errDuplicatePath(col.routes[i].Method + " " + col.routes[i].Path)
			}
```

`Mount` gains the publisher (`events.Publisher`, nil = nobody listens) and no longer sets a module:

```go
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
```

Update the package comments: `DefaultPrefix` is "where operations are mounted: `<METHOD> /api/<module>/<entity>`", and `Routes`/`Mount` docs. `gotest` compiles the server tests.

## Stage 4 — the client (`caller.go`)

Replace `Dispatch`, `Call`, `CallKeyed`, `send` and the constants; the response handling inside `Send(func(resp, err){…})` is copied **unchanged**.

```go
const (
	msgKeyRequired = "rpc: idempotency key is required"
	msgKeyedRead   = "rpc: a read has no idempotency key; use Call"
	msgBadOp       = "rpc: invalid operation: "
)

// Dispatch is Call without a result: fire and forget.
func (c *Caller) Dispatch(op router.Operation, args model.Encodable) {
	c.Call(op, args, nil, nil)
}

// Call sends the operation as its REST request (see requestOf) and decodes a 2xx answer into into. A Read travels in the query,
// the other actions as a JSON body. Any other outcome reaches done as an *Error.
func (c *Caller) Call(op router.Operation, args model.Encodable, into model.Decodable, done func(err error)) {
	method, path, ok := requestOf(DefaultPrefix, op)
	if !ok {
		c.fail(done, msgBadOp+op.String())
		return
	}
	if !hasBody(method) { // GET: the arguments are the query
		query := ""
		if args != nil && !args.IsNil() {
			q, err := router.EncodeQuery(args)
			if err != nil {
				c.fail(done, err.Error())
				return
			}
			query = q
		}
		c.send(method, c.origin+path, query, "", nil, into, done)
		return
	}
	var body []byte
	if args == nil || args.IsNil() {
		body = []byte("{}")
	} else if err := json.Encode(args, &body); err != nil {
		c.fail(done, err.Error())
		return
	}
	c.send(method, c.origin+path, "", "", body, into, done)
}

// CallKeyed is Call with the request header router.HeaderIdempotencyKey set to key, so a server running the idempotency middleware
// answers a repeated send once. A read has no key (it is safe to repeat): CallKeyed refuses it. key == "" → done receives an *Error
// with Status 0 and nothing is sent. body is the operation's args already encoded as JSON (as a queue stores them); empty sends "{}".
func (c *Caller) CallKeyed(op router.Operation, key string, body []byte, into model.Decodable, done func(err error)) {
	method, path, ok := requestOf(DefaultPrefix, op)
	if !ok {
		c.fail(done, msgBadOp+op.String())
		return
	}
	if !hasBody(method) {
		c.fail(done, msgKeyedRead)
		return
	}
	if key == "" {
		c.fail(done, msgKeyRequired)
		return
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	c.send(method, c.origin+path, "", key, body, into, done)
}

func (c *Caller) fail(done func(err error), text string) {
	if done != nil {
		done(&Error{Status: 0, Body: text})
	}
}

func (c *Caller) send(method, url, query, key string, body []byte, into model.Decodable, done func(err error)) {
	if query != "" {
		url += "?" + query
	}
	var req *fetch.Request
	switch method {
	case methodGet:
		req = fetch.Get(url)
	case methodPost:
		req = fetch.Post(url)
	case methodPatch:
		req = fetch.Patch(url)
	default:
		req = fetch.Delete(url)
	}
	if key != "" {
		req.Header(router.HeaderIdempotencyKey, key)
	}
	if hasBody(method) {
		req = req.ContentTypeJSON().Body(body)
	}
	req.Send(func(resp *fetch.Response, err error) {
		// …the existing body of this callback, unchanged (error → *Error, non-2xx → *Error with Problem detail, 2xx → decode into)…
	})
}
```

Delete `msgBadOpName` and the dot-scanning code. `var _ router.Caller = (*Caller)(nil)` stays. `gotest` passes now: every test of stage 1 is green.
Commit (stages 2–4 together, one compiling unit): `refactor!: REST operations from router.Operation`.

## Stage 5 — translations and documentation

- `langc sync .`: fill every empty value (all languages in `lang.json`); zero untranslated, zero reuse warnings (new texts: `msgKeyedRead`, `msgBadOp`, `errInvalidOperation`'s words).
- `README.md` (≤ 150 lines, four ordered sections): *Why This Exists* Without/With (hidden `_c` suffix vs method + resource); *Step-by-Step Guide* a compilable example: mount with `rpc.Mount(r, rpc.DefaultPrefix, nil, nil, broker, mod)` and call with `rpc.NewCaller("").Call(router.Operation{…}, args, &out, done)`, plus the table of the Goal section.
- `docs/ARCHITECTURE.md`: four sections; the mapping, `Cache-Control: no-store`, the JSON requirement for mutations, why a read has no idempotency key.
- `docs/diagrams/REST_FLOW.md`: three parts (context header, `flowchart TD` of op → `requestOf` → method/path → handler → `guard`, numbered walkthrough).
- `docs/TESTS.md`: remove the row of `TestRPCBadOp`, add one row per new test.
- Delete the old `docs/LAST_PLAN_EXECUTED.md` content is not touched; do not link `docs/PLAN.md` from permanent docs.

Commit: `docs: REST operations`.

## Delivery check

1. `gotest` in the module and in `tests/` (a PR that does not compile, or whose tests could not run, is a failed delivery).
2. `langc sync .` → zero untranslated, zero reuse warnings.
3. `grep -rn "_c\"\|_r\"\|_u\"\|_d\"\|msgBadOpName\|crud\.Operation(" --include=*.go .` → empty.
4. `git diff --stat main` lists only: `request.go`, `mount.go`, `caller.go`, `lang.json`, `tests/*`, `README.md`, `docs/*`.
5. Consumers (`appkit`, `dbsync`, `mcp`, `view`, `layout`) change in their own plans; `rpc.Mount` gained a parameter.

| Stage | Files | Commit |
|---|---|---|
| 1 | `tests/*` | `test: … (red)` |
| 2–4 | `request.go`, `mount.go`, `caller.go` | `refactor!: …` |
| 5 | `lang.json`, `README.md`, `docs/*` | `docs: …` |
