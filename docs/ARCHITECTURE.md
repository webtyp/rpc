# rpc: domain operations over plain HTTP

This package provides an HTTP transport binding for `crud.Module` operations mounted via `crud.Mount`.

Each operation becomes its own route, with the permission, arguments, and description derived by `crud.Mount`, served by any `router.Router` implementation, and callable from client applications or browsers.

## Prior art

- **Connect RPC** (Buf): `POST /<package>.<Service>/<Method>`, JSON body, status codes mapped from errors, no REST verb mapping. Our shape.
- **Twirp** (Twitch): `POST /twirp/<package>.<Service>/<Method>` with JSON; deliberately not REST, one URL per procedure.
- **tRPC**: `/trpc/<router>.<procedure>`; a typed client generated from the server's procedures. Same idea as our `Caller`, without code generation because the args are already typed models.
- **REST (Rails resources, JAX-RS)**: rejected. Our operations are typed domain operations, and mapping them onto arbitrary HTTP paths loses contract guarantees.

## Errors

Error writing is centralized in `router.WriteError`, adhering to RFC 9457 (Problem Details for HTTP APIs). Unclassified errors (500) hide internal error text from clients, logging the real error server-side via `router.InternalError`.

## Security Rule: 415/CSRF

The `Content-Type` header of all requests must exactly be `application/json`.
If it is not, the request fails with 415 Unsupported Media Type without calling the handler.

## Example

```go
// server (composition root)
if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, logFunc, nil, patients); err != nil {
    return err
}

// client
caller := rpc.NewCaller("")
var list PatientList
caller.Call("patient_directory."+crud.Op(&Patient{}, model.Read), &crud.ReadArgs{Limit: 20}, &list, done)
```
