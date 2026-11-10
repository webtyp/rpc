# Architecture: rpc

HTTP transport binding for domain operations mounted via `crud.Mount`. Operations travel as REST requests where the HTTP method is the action and the path is the resource.

## 1. Overview in One Minute

`rpc` connects domain modules (`crud.Module`) to HTTP networks. Instead of hiding verbs in URL suffixes (like `_c` or `_r`), each `router.Operation` maps directly to standard REST semantics:

| `router.Operation.Action` | Method | Path | Arguments |
|---|---|---|---|
| `model.Create` | `POST` | `/api/<module>/<entity>` | JSON body `{records}` |
| `model.Read` | `GET` | `/api/<module>/<entity>` | Query string (`id`, `search`, `after`, `limit`, `filter.<field>`) |
| `model.Update` | `PATCH` | `/api/<module>/<entity>` | JSON body `{ids, fields, record}` |
| `model.Delete` | `DELETE` | `/api/<module>/<entity>` | JSON body `{ids}` |

The mapping lives in `requestOf` in `request.go` and is shared between server mounting (`rpc.Mount`) and client calling (`rpc.Caller`), guaranteeing the client and server cannot drift apart.

## 2. Repository Pieces & On-Disk Layout

| File | Responsibility |
|---|---|
| `request.go` | Canonical mapping between `router.Operation` and HTTP method/path (`MethodOf`, `requestOf`). |
| `mount.go` | Server registration (`rpc.Mount`, `rpc.Routes`) adapting `crud.Module` onto `router.Router`. |
| `caller.go` | Client `router.Caller` implementation (`rpc.NewCaller`) executing REST requests via `webtyp.com/fetch`. |
| `errors.go` | Wire error decoding and mapping to `model.ErrorKind`. |

## 3. Step-by-Step Flows

See [REST Flow Diagram](diagrams/REST_FLOW.md) for the visual flowchart.

### The REST Mapping & Query Codec
When a client calls `Caller.Call(op, args, into, done)`:
- If `op.Action == model.Read` (`GET`), arguments are encoded as URL query parameters via `router.EncodeQuery`. Filter fields become `filter.<field>` query keys. No request body or `Content-Type` is sent.
- If `op.Action` is a mutation (`POST`, `PATCH`, `DELETE`), arguments are encoded as a JSON payload using `webtyp.com/json`.

On the server, `mock.Router` / `routertest.Serve` decodes arguments via `router.DecodeArgs`: reads query params for `GET` and JSON body for mutations.

### Why a Read Has No Idempotency Key
Idempotency keys (`router.HeaderIdempotencyKey`) exist to prevent duplicate mutations when network retries occur. A read (`GET`) is inherently idempotent and safe to repeat without side effects; setting an idempotency key on a read makes no sense. Consequently, `Caller.CallKeyed` explicitly refuses any operation with `model.Read` with a client-side error before network transmission.

## 4. Security Boundaries & Contracts

### Cache-Control: no-store
Every response from an RPC endpoint carries `Cache-Control: no-store`. Sensitive business data (such as medical or user records) must never be cached by shared proxies, intermediary CDNs, or browser caches.

### JSON Requirement for Mutations (CSRF Mitigation)
All mutation methods (`POST`, `PATCH`, `DELETE`) require `Content-Type: application/json`. Any cross-site form submission (`application/x-www-form-urlencoded` or `multipart/form-data`) is rejected immediately by the `guard` middleware with HTTP 415 Unsupported Media Type before reaching domain handlers. Because standard browser HTML forms cannot post JSON without preflight cross-origin restrictions, this provides strong CSRF protection. A `GET` carries no body and requires no content type.
