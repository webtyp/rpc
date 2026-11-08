---
PLAN: "refactor!: rpc.Routes and rpc.Mount take []crud.Public — the app opens reads and writes to visitors"
EXECUTOR: antigravity
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit after every stage and stop. Never ask whether to
> proceed. This plan runs **locally** and is **published as soon as the delivery check passes** (no `replace` anywhere).
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.

# Plan: public actions declared by the app

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/crud@latest
```
`crud@latest` must contain `crud.Public` and `crud.Mount(reg, public []crud.Public, m)` (plan A6.2); if `go doc webtyp.com/crud Public`
fails, stop.

## Why

Decision D23 of `webtyp/docs/ARCHITECTURE_MASTER_PLAN.md`: a patient books and pays online without a session, so the app must
open writes too, not only reads (D10). The list keeps living in the app's `config/`; `rpc` only passes it to `crud.Mount`.

## Stage 1 — `mount.go`

- `func Routes(prefix string, public []crud.Public, modules ...crud.Module) ([]router.RouteInfo, error)` (was `[]model.ModuleNaming`).
- `func Mount(r router.Router, prefix string, log func(...any), public []crud.Public, modules ...crud.Module) error`.
- The doc comment of `Mount`: «Public actions come from the app (`crud.Public`), never from a module.»
- Every call site in this repo and its tests adapts (`grep -rn "Routes(\|Mount(" --include=*.go .`). Callers that pass `nil` need no change.

## Stage 2 — tests (rows in `docs/TESTS.md`)

| Test | Expected |
|---|---|
| `TestRoutes_PublicCreate` | a module with a table allowing Create; `public = [{Model: &T{}, Actions: model.Create}]` → the `_c` route has `Access == model.AccessPublic`, the `_r` route does not |
| `TestRoutes_PublicReadStillWorks` | `[{Model: &T{}, Actions: model.Read}]` → `_r` public (the D10 behaviour) |

## Delivery check

`gotest`. A tree that does not compile or whose tests did not run is a failed delivery.

| Stage | Files |
|---|---|
| 1 | `mount.go`, tests |
| 2 | `tests/*`, `docs/TESTS.md` |
