# rpc: domain operations over plain HTTP
<img src="docs/img/badges.svg">

HTTP transport binding for `crud.Module` operations mounted via `crud.Mount`.

Each operation becomes its own POST route (`POST <prefix>/<Module>/<op>`), with the permission, arguments, and description derived by `crud.Mount`.

## Why This Exists

Provides a clean JSON RPC transport over HTTP without inventing REST verb mappings. Operates directly on `crud.Module` instances and routes requests through centralized validation.

## Before You Start

- Domain modules implement `crud.Module` (`ModelName() string` and `MountOperations(reg crud.Registry)`).
- Public reads are declared explicitly by the application composition root via `public []model.ModuleNaming`.
- Errors over HTTP are written via [`router.WriteError`](https://github.com/webtyp/router).

## Step-by-Step Guide

Mount operations on the server:

```go
// server (composition root)
if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, logFunc, nil, patients); err != nil {
    return err
}

// client (in browser or caller)
caller := rpc.NewCaller("")
var list PatientList
caller.Call("patient_directory."+crud.Op(&Patient{}, model.Read), &crud.ReadArgs{Limit: 20}, &list, func(err error) {
    if err != nil {
        log.Println(err)
    }
})
```

## Security & Errors

- **415 / CSRF:** All requests must send `Content-Type: application/json`. Non-JSON requests are rejected with 415.
- **Problem Details:** Handlers and `crud.Mount` write RFC 9457 responses via `router.WriteError`. Client calls return `*rpc.Error` compatible with `model.KindOf(err)` and `model.FieldOf(err)`.
- **Idempotency:** `caller.CallKeyed(op, key, body, into, done)` sends the `Idempotency-Key` header for safe retries.

## Documentation Map

| Document | Purpose |
|---|---|
| [`mount.go`](mount.go) | `rpc.Mount` and `rpc.Routes` mounting `crud.Module` |
| [`caller.go`](caller.go) | Client `rpc.Caller` implementation |
| [`errors.go`](errors.go) | Client error decoding and kind reporting |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Architectural details and wire patterns |
| [`docs/TESTS.md`](docs/TESTS.md) | Test suite map |
