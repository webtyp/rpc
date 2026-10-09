---
PLAN: "feat!: NewCaller returns *rpc.Caller with CallKeyed — send an operation with an Idempotency-Key"
TAG: v0.2.0
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 11570812058643988654
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `rpc`: call an operation with an idempotency key

## 0. Context (read first)

`webtyp.com/rpc` v0.1.0 binds domain operations to HTTP: `rpc.Mount(r, prefix, modules...)` serves
each one as `POST /api/<module>/<op>`, and `rpc.NewCaller(origin) router.Caller` calls them from
the browser (`caller.go`, using `webtyp.com/fetch`).

The offline-first sync engine (`webtyp/dbsync`, next plan) replays operations queued while the
server was unreachable. A replay must carry the request header `Idempotency-Key: <mutation id>` so
the server (`webtyp.com/idempotency` middleware) answers a repeated send once. The header name is
`router.HeaderIdempotencyKey` in `webtyp.com/router` **v0.4.1**. Today the caller cannot send any
header.

Go idiom: accept interfaces, return structs. Returning the concrete `*Caller` lets callers that need
the extra method use it, while it still satisfies `router.Caller` for every view.

## Design gate

### 1. Prior art
- **Stripe Go client** (`params.IdempotencyKey`): the key is an explicit per-call argument.
- **AWS SDK** (`ClientToken` on create calls) and **Google Cloud** (`request_id`): idempotency token
  passed explicitly with the call it protects.
- **Go standard library** (`net/http.NewRequest` returns `*http.Request`; constructors return
  concrete types).

### 2. Novice-name test
`caller.CallKeyed(op, key, args, into, done)` — "call this operation, with this idempotency key".
`rpc.Caller` — "the rpc caller".

### 3. Complexity ledger
```
Concepts the developer must learn   +1 (CallKeyed)
Files they must touch to do X        0
Lines at the call site               0 for every existing NewCaller user (still a router.Caller)
Ways to do the same thing            0 (Call = no key; CallKeyed = with key: different intents)
```

### 4. Where it belongs
Here: building the HTTP request for an operation is this library's concern. The header name comes
from `router` (shared with the server-side middleware, which this library must not import).

### 5. What this deletes
The unexported `clr` type (replaced by the exported `Caller`); the `router.Caller` return type of
`NewCaller` (now `*Caller`).

## 1. Target API (changes only)

```go
// Caller sends operations over HTTP. It implements router.Caller.
type Caller struct{ /* unexported: origin */ }

// NewCaller returns a Caller for origin ("" = the page's own origin). Uses DefaultPrefix.
func NewCaller(origin string) *Caller

func (c *Caller) Call(op string, args model.Encodable, into model.Decodable, done func(err error))
func (c *Caller) Dispatch(op string, args model.Encodable)

// CallKeyed is Call with the request header router.HeaderIdempotencyKey set to key, so a server
// running the idempotency middleware answers a repeated send once. key == "" → done receives an
// *Error with Status 0 and Body "rpc: idempotency key is required" and nothing is sent.
func (c *Caller) CallKeyed(op, key string, args model.Encodable, into model.Decodable, done func(err error))

var _ router.Caller = (*Caller)(nil)
```

`Call` and `CallKeyed` share one unexported function that builds and sends the request (the only
difference is the header); no duplicated request code. Error semantics (`*rpc.Error{Status, Body}`)
unchanged.

## 2. Stages

| Stage | Files | Content |
|---|---|---|
| 1 | `go.mod`, `tests/go.mod` | `go get webtyp.com/router@v0.4.1` in both modules |
| 2 | `caller.go` | §1 |
| 3 | `tests/rpc_test.go` | §3 |
| 4 | `README.md`, `docs/ARCHITECTURE.md` | "I want to retry safely → `CallKeyed`"; note that the server needs `webtyp.com/idempotency` |

## 3. Tests (existing nested `tests/` module, real `httpd` server as today)

1. `CallKeyed("testmod.echo", "k1", args, &out, done)` → the handler sees header
   `Idempotency-Key: k1` (read it in the test handler with `ctx.GetHeader(router.HeaderIdempotencyKey)`)
   and `out` equals `args`.
2. `Call` sends **no** `Idempotency-Key` header.
3. `CallKeyed` with an empty key → `*rpc.Error{Status: 0}` with the message, and the handler did not run.
4. All existing tests stay green unchanged (`NewCaller` still usable as `router.Caller`).

## 4. Code rules (non-negotiable)
- WASM-compatible: `webtyp.com/fmt`; no `errors`, `strconv`, `strings`, `encoding/json`, `net/http`
  in library code; no `map`. The header name is `router.HeaderIdempotencyKey`, never a literal.
- No exported symbol beyond §1. Tests in `tests/`; never export for a test.

## 5. Acceptance criteria
- `gotest ./...` green.
- `grep -rn "\"Idempotency-Key\"" --include=*.go .` → empty (only the router constant is used).
- **Before opening the PR:** `git diff --stat main` lists every file of the stages table, and no file
  is deleted that this plan does not order deleted.
