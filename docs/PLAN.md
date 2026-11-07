---
PLAN: "refactor!: rpc mounts crud.Module through crud.Mount; WriteError moves to router"
EXECUTOR: antigravity
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit after every stage and stop. Never ask whether to
> proceed. This plan runs **locally**: do not tag, push or publish anything; the owner reviews first.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.

# Plan: `rpc` accepts only `crud.Module`

Context: master `webtyp/docs/ARCHITECTURE_MASTER_PLAN.md` D5, D7, D10, D14. Runs after the router and crud plans.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```
`go.mod`: `replace webtyp.com/model => ../model`, `replace webtyp.com/router => ../router`, `replace webtyp.com/crud => ../crud`; `go get webtyp.com/crud` then `go mod tidy`.

## APIs used

| Symbol | Signature | Where |
|---|---|---|
| `crud.Module` | `interface{ model.ModuleNaming; MountOperations(reg crud.Registry) }` | crud `contract.go` (local) |
| `crud.Mount` | `func Mount(reg router.OperationRegistry, public []model.ModuleNaming, m crud.Module) error` | crud `mount.go` (local) |
| `crud.Loopback` | `func Loopback(mods ...crud.Module) router.Caller` | crud `loopback.go` (local) |
| `router.WriteError` / `router.InternalError` | `func WriteError(ctx Context, err error)` / `func InternalError(ctx Context) string` | router `problem_write.go` (local) |

If a signature differs, stop and name it.

## Stage 1 — delete `WriteError`

- `errors.go`: delete `WriteError`, `contentTypeProblem`, `msgInternal`, `ctxKeyInternal` (keep `Error` and its methods: the client still reports problems).
- `mount.go` `guard`: replace `c.Value(ctxKeyInternal)` with `router.InternalError(c)`.
- Delete `tests/write_error_test.go` (ported to router by the router plan).
- Grep: `grep -rn "WriteError\|ctxKeyInternal" --include=*.go .` → empty.

## Stage 2 — `Routes` and `Mount` take `crud.Module` and the public reads

New signatures (`mount.go`):
```go
func Routes(prefix string, public []model.ModuleNaming, modules ...crud.Module) ([]router.RouteInfo, error)
func Mount(r router.Router, prefix string, log func(...any), public []model.ModuleNaming, modules ...crud.Module) error
```
In both loops replace `m.MountOperations(col)` / `m.MountOperations(act)` with
`if err := crud.Mount(col, public, m); err != nil { return nil, err }` (`return err` in `Mount`).
`collectingRegistry` and `activeRegistry` keep implementing `router.OperationRegistry` unchanged.
Update the doc comments: public reads come from the app (`appkit` `AppServer.PublicReads`), never from a module.

## Stage 3 — tests

Every test module in `tests/` becomes a `crud.Module` (`MountOperations(reg crud.Registry)` with `reg.Read`/`reg.Create`…);
every `router/loopback` import → `crud.Loopback`. New tests in `tests/mount_public_test.go`:
- `TestRoutes_PublicReadFromApp`: `rpc.Routes("/api", []model.ModuleNaming{&rec{}}, mod)` → the route `/api/<mod>/rec_r` has `Access == model.AccessPublic`; `/api/<mod>/rec_c` has `Resource == "rec"`, `Action == model.Create`.
- `TestRoutes_NoPublicByDefault`: `public` nil → `rec_r` is `AccessGuarded` with `Resource == "rec"`, `Action == model.Read`.
- `TestMount_CrudWiringErrorStopsMount`: module calling `reg.Delete(&rec{}, nil)` → `Mount` returns `crud.ErrNilFunction`.
Add the rows to `docs/TESTS.md`.

## Stage 4 — documentation

`README.md` (4 mandatory sections, ≤150 lines) and `docs/*`: show `rpc.Mount(r, rpc.DefaultPrefix, log, nil, patients)`, link `router.WriteError`, remove every mention of `OperationModule` and `rpc.WriteError`.

## Delivery check

`gotest` passes; `grep -rn "OperationModule\|router/loopback\|WriteError" --include=*.go .` → only `router.WriteError` mentions in comments, if any.

| Stage | Files |
|---|---|
| 1 | `errors.go`, `mount.go`, `tests/write_error_test.go` (deleted) |
| 2 | `mount.go` |
| 3 | `tests/*.go`, `docs/TESTS.md` |
| 4 | `README.md`, `docs/*` |
