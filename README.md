# rpc: domain operations over plain HTTP
<img src="docs/img/badges.svg">

This package provides an HTTP binding for `router.OperationModule`.

Each operation becomes its own route, with the permission, arguments, and description the module declared, so it is visible in `Router.Routes()`, served by any `router.Router` implementation, and callable from the browser.

## Prior art

- **Connect RPC** (Buf): `POST /<package>.<Service>/<Method>`, JSON body, status codes mapped from errors, no REST verb mapping. Our shape.
- **Twirp** (Twitch): `POST /twirp/<package>.<Service>/<Method>` with JSON; deliberately not REST, one URL per procedure.
- **tRPC**: `/trpc/<router>.<procedure>`; a typed client generated from the server's procedures. Same idea as our `Caller`, without code generation because the args are already typed models.
- **REST (Rails resources, JAX-RS)**: rejected. Our operations are named commands (`change_reservation_status`, `overbook`), and mapping them onto CRUD verbs loses meaning and duplicates decisions the module already made.

## Security Rule: 415/CSRF

The `Content-Type` header of all requests must exactly be `application/json`.

If it is not, the request will fail with a `415` HTTP status code and an error body `rpc: Content-Type must be application/json` without calling the handler.

**Why**: the native server also decodes HTML form bodies; a cross-site `<form method=post>` cannot send `application/json` without a CORS preflight, so this closes CSRF for cookie-authenticated operations.

## Example

```go
// server (composition root)
if err := rpc.Mount(srv.Router(), rpc.DefaultPrefix, booking, patients, records); err != nil {
    return err
}
// browser (composition root)
caller := rpc.NewCaller("")
view, _ := patientsui.Browser(caller, ids, tenantID)
```

## Retrying safely (Idempotency)

If you need to retry an operation safely, use `CallKeyed`. It works like `Call`, but allows you to set an explicit `Idempotency-Key` header (using `router.HeaderIdempotencyKey` as the header name).

```go
caller.CallKeyed("booking.overbook", "unique-request-id-123", body, &resp, done)
```

Note: To actually benefit from this, your server must be running the `webtyp.com/idempotency` middleware, which uses this header to ensure a repeated send is only executed once.
