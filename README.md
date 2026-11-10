# rpc: domain operations over plain HTTP
<img src="docs/img/badges.svg">

HTTP transport binding for [`crud.Module`](https://github.com/webtyp/crud) operations mounted via `rpc.Mount`.

## Why This Exists

`rpc` exposes domain operations as clean REST endpoints without manual route boilerplate. Both the client ([`Caller`](caller.go)) and the server ([`Mount`](mount.go)) share one canonical mapping ([`requestOf`](request.go)) from [`router.Operation`](https://github.com/webtyp/router), eliminating endpoint drift between frontend and backend.

### Without this vs. With this

- **Without this:** Every operation hides its verb in an ad-hoc URL suffix (such as `POST /api/agenda/patient_r` for reads and `POST /api/agenda/patient_c` for creates). Standard HTTP caching, proxies, and REST conventions are broken, while client and server URL parsers frequently diverge.
- **With this:** The HTTP method is the action and the path is the resource (`GET /api/agenda/patient`, `POST /api/agenda/patient`). One shared mapping governs client calls and server routes, mutations enforce JSON to prevent CSRF, and all responses carry `Cache-Control: no-store`.

## Before You Start

- Domain models define schema and permissions with [`webtyp.com/model`](https://github.com/webtyp/model).
- Domain modules implement `crud.Module` with `Definition()` and `MountOperations(reg crud.Registry)`.
- Client and server routers use [`webtyp.com/router`](https://github.com/webtyp/router).

## Step-by-Step Guide

### REST Operation Mapping

| `router.Operation.Action` | Method | Path | Arguments |
|---|---|---|---|
| `model.Create` | `POST` | `/api/<module>/<entity>` | JSON body `{records}` |
| `model.Read` | `GET` | `/api/<module>/<entity>` | Query string (`id`, `search`, `after`, `limit`, `filter.<field>`) |
| `model.Update` | `PATCH` | `/api/<module>/<entity>` | JSON body `{ids, fields, record}` |
| `model.Delete` | `DELETE` | `/api/<module>/<entity>` | JSON body `{ids}` |

### Step 1: Mount module operations and call via REST

Mount module operations on any [`router.Router`](https://github.com/webtyp/router) and execute calls using [`rpc.Caller`](caller.go):

```go
package main

import (
	"webtyp.com/crud"
	"webtyp.com/events"
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
	"webtyp.com/router/mock"
	"webtyp.com/rpc"
)

func run(r router.Router, broker events.Publisher, mod crud.Module) error {
	// Mount operations on the router
	if err := rpc.Mount(r, rpc.DefaultPrefix, nil, nil, broker, mod); err != nil {
		return err
	}

	// Client caller sending a Read operation
	caller := rpc.NewCaller("")
	readOp := router.Operation{Module: mod.ModelName(), Entity: "patient", Action: model.Read}
	args := &crud.ReadArgs{Search: "123", Limit: 20}

	caller.Call(readOp, args, nil, func(err error) {
		if err != nil {
			fmt.Println("call failed:", err)
			return
		}
		fmt.Println("call succeeded")
	})
	return nil
}

func main() {
	var r *mock.Router
	_ = r
}
```

## Documentation Map

| Order | Document | What question it answers & when to read it |
|---|---|---|
| 1 | [README.md](README.md) | How to mount and call operations over HTTP. |
| 2 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Details on REST mapping, `Cache-Control: no-store`, CSRF mitigation, and idempotency rules. |
| 3 | [docs/diagrams/REST_FLOW.md](docs/diagrams/REST_FLOW.md) | Sequence diagram of requests flowing through `requestOf`, router, and guard middleware. |
| 4 | [docs/TESTS.md](docs/TESTS.md) | Inventory of all unit and integration tests and what each verifies. |
