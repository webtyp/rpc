---
PLAN: "feat: WriteError — one answer for every operation's error: status from model.KindOf, the failed field, a generic 500 with the detail in the log"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 322685870644420113
---

> Read AGENTS.md at the repo root before any change.

# PLAN — `rpc.WriteError`

## Why

Every domain module answers its errors with its own `errors.go` (`domainError` + `statusOf` +
`writeError`), and they drifted: `device_manager` and `staff_manager` answer 400 with an **empty
body**; `service_catalog` writes `err.Error()` even on a 500, so SQL text reaches the browser; and
no screen can mark the field that failed. Decision D22 of
`veltylabs/mjosefa-cms/docs/MVP_DEPLOYMENT_ROADMAP.md`: one piece answers every operation's error.
`webtyp.com/model` v0.2.10 published the vocabulary (`model.KindOf`, `model.FieldOf`,
`model.NotFound`/`Conflict`/`Invalid`, `*model.FieldError`). This plan is the piece; the next ones
make each module delete its `errors.go`.

## Design gate (api-design)

1. **Prior art.** RFC 9457 Problem Details (https://www.rfc-editor.org/rfc/rfc9457.html):
   `status`, `title` (short, per problem type), `detail` (this occurrence), extension members for
   the invalid field, and «avoid exposing implementation internals through error messages».
   gRPC status codes (https://grpc.io/docs/guides/status-codes/): a fixed set the framework maps.
   Kubernetes `apimachinery/pkg/api/errors` (https://pkg.go.dev/k8s.io/apimachinery/pkg/api/errors):
   components build typed errors, the API server picks the HTTP status.
2. **Names.** `rpc.WriteError(ctx, err)` — «write this error as the answer». `Error.Field`,
   `Error.Detail` on the client side.
3. **Ledger.** Concepts +1 / −N (each module's `statusOf`/`writeError`); call sites: one line per
   handler, as today; ways to answer an error: N → 1.
4. **Where.** `rpc`: it already owns the operation transport (`Mount`, `Caller`, `Error`).
5. **Deletes.** Nothing here; the module plans delete their `errors.go`.

## APIs used

| Symbol | Exact | Tag |
|---|---|---|
| `model.KindOf` | `func KindOf(err error) ErrorKind` | model v0.2.10 |
| `model.FieldOf` | `func FieldOf(err error) string` | model v0.2.10 |
| `model.ErrorInternal`, `ErrorInvalid`, `ErrorNotFound`, `ErrorConflict` | `ErrorKind` constants; `ErrorKind.String()` → `"internal"`, `"invalid"`, `"not found"`, `"conflict"` | model v0.2.10 |

## Tools

```
go get webtyp.com/model@v0.2.10
```

## Stage 1 — `errors.go`: `WriteError`

```go
// WriteError answers an operation's error. The status comes from model.KindOf(err):
// invalid 400, not found 404, conflict 409, anything unclassified 500. The body is an
// RFC 9457 problem (Content-Type application/problem+json):
//
//	{"status":400,"title":"invalid","detail":"name required","field":"name"}
//
// A 500 never carries err's text: its detail is msgInternal and the real error is
// logged by Mount's logger.
//
//	if err != nil { rpc.WriteError(ctx, err); return }
func WriteError(ctx router.Context, err error)
```

- Constants (named, in `errors.go`): `contentTypeProblem = "application/problem+json"`,
  `msgInternal = "internal error"`, `ctxKeyInternal = "rpc.internal"` (unexported context key).
- Status mapping as above (`model.ErrorInvalid` → 400, `ErrorNotFound` → 404,
  `ErrorConflict` → 409, default 500).
- Body: an unexported `problem struct{ Status int64; Title, Detail, Field string }` with a
  hand-written `EncodeFields(w model.FieldWriter)` (`status`, `title`, `detail`, and `field` only
  when not empty) and `IsNil() bool`. `Title` = `kind.String()`. `Detail` = `err.Error()`, except
  500 → `msgInternal`. `Field` = `model.FieldOf(err)`.
- Write: `ctx.SetHeader(contentTypeName, contentTypeProblem)`, `ctx.WriteStatus(status)`,
  `ctx.Encode(&p)` (use `ctx.Encode`, the transport's codec; if `Encode` does not set the body
  as JSON in this router version, encode with `json.Encode` and `ctx.Write`).
- For a 500 also `ctx.SetValue(ctxKeyInternal, err.Error())`.

## Stage 2 — `mount.go`: the logger

- `Mount` gets the logger as its third parameter:
  `func Mount(r router.Router, prefix string, log func(...any), modules ...router.OperationModule) error`.
  `nil` = no log. Update its doc comment.
- `activeRegistry` stores `log`; `guard(h)` becomes `guard(h, path, log)`: after `h(c)`, if
  `d := c.Value(ctxKeyInternal); d != "" && log != nil`, call `log("rpc:", path, "internal error:", d)`.
- Update every call in this repo's tests to pass `nil`.

## Stage 3 — `caller.go` + `errors.go`: the client reads the problem

- `Error` gets two fields: `Detail string // the problem's detail, when the server sent one` and
  `Field string // the field that failed, "" when none`.
- In `send`, when the status is outside 2xx: keep `Body: resp.Text()` and, if the body starts with
  `{`, decode it with `json.Decode` into the `problem` type (add `DecodeFields(r model.FieldReader)`)
  and copy `Detail` and `Field`. A body that does not decode leaves them empty (no error).
- `(*Error).Error()`: when `Detail != ""` return `fmt.Sprintf("rpc: %d %s", e.Status, e.Detail)`;
  otherwise as today. So a toast shows «rpc: 400 name required», never raw JSON.

## Stage 4 — `tests/write_error_test.go` (new)

Through the real stack: mount a test `OperationModule` with operations that return each error
kind via `rpc.WriteError`, on `httpd` or the router used by `tests/rpc_test.go` (reuse its setup),
and call them with `rpc.NewCaller(ts.URL)` where the existing tests do; otherwise assert on the
HTTP response.

| Test | Operation returns | Expected |
|---|---|---|
| `TestWriteError_NotFound` | `model.NotFound("service not found")` | 404; `*rpc.Error` with `Detail` `service not found`, `Field` "" |
| `TestWriteError_Conflict` | `model.Conflict("in use")` | 409 |
| `TestWriteError_FieldError` | `&model.FieldError{Field: "name", Err: fmt.Err("name", "required")}` | 400; `Field` = `name`; `Error()` = `rpc: 400 name required` |
| `TestWriteError_InternalHidesDetail` | `fmt.Err("pq: relation x does not exist")` | 500; body does **not** contain `pq:`; `Detail` = `internal error`; the logger passed to `Mount` received a line containing `pq: relation x does not exist` |
| `TestWriteError_ProblemContentType` | any | `Content-Type` is `application/problem+json` |
| `TestCaller_PlainBodyStillWorks` | a handler that writes 400 with plain text `bad` | `Body` = `bad`, `Detail` "", `Error()` as before |

## Stage 5 — docs

- `README.md`: a section `## Errors` — the mapping table, the body shape, the one-line usage, the
  500 rule, the client fields, and the new `Mount` logger parameter.
- `docs/` architecture file if present: why one piece (the drift between modules) and why the 500
  hides its detail (RFC 9457 §4).

## Delivery check

```
gotest
```

- A PR that does not compile, or whose tests could not run, is a failed delivery.
- Never pause to ask permission: when the check passes, commit, push and open the PR. Stuck 30
  minutes: open the PR listing what fails.
- `git diff --stat main` lists only: `errors.go`, `mount.go`, `caller.go`, `tests/*`, `README.md`,
  the docs file, `go.mod`, `go.sum`, this plan.
- `Mount`'s signature changes: consumers outside this repo (`webtyp/appkit`, `webtyp/dbsync/tests`)
  are updated by the orchestrator after the merge, not in this PR.

## Stages

| # | Files | Done when |
|---|---|---|
| 1 | `errors.go`, `go.mod`, `go.sum` | compiles |
| 2 | `mount.go`, existing tests | suite green |
| 3 | `caller.go`, `errors.go` | compiles |
| 4 | `tests/write_error_test.go` | 6 tests pass |
| 5 | `README.md`, docs | section present |
