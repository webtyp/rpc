# Test Suite

Summary of tests covering `webtyp.com/rpc`. Tests run against standard library `routertest.Serve` in front of `router/mock`.

## Test Inventory

| Test | Location | Verifies |
|---|---|---|
| `TestRPCEcho` | `tests/rpc_test.go` | `Read` operation round-trip decodes response into result slice. |
| `TestRPCCallKeyed` | `tests/rpc_test.go` | `CallKeyed` sends `Idempotency-Key` header with mutation. |
| `TestRPCCallKeyedEmptyKey` | `tests/rpc_test.go` | `CallKeyed` with empty key fails client-side with Status 0. |
| `TestRPCRoutes` | `tests/rpc_test.go` | Route registration methods and paths on `router.Router`. |
| `TestRPCSecretUnauthorized` | `tests/rpc_test.go` | Unauthorized request receives 4xx HTTP error status. |
| `TestRPCCSRF` | `tests/rpc_test.go` | Form post (`application/x-www-form-urlencoded`) to POST, PATCH, DELETE rejected with 415. |
| `TestRPCConflict` | `tests/rpc_test.go` | Server conflict error decoded into `*rpc.Error` with Status 409 and Problem detail. |
| `TestRPCUnreachable` | `tests/rpc_test.go` | Unreachable endpoint returns Status 0 network error. |
| `TestRPCMountErrors` | `tests/rpc_test.go` | Invalid prefix, empty module name, and duplicate routes rejected at mount. |
| `TestRoutes_Declarations` | `tests/routes_test.go` | `rpc.Routes` produces expected methods, paths, MFA, and descriptions without a server. |
| `TestRoutes_EmptyModelName` | `tests/routes_test.go` | Module with empty `ModelName()` rejected with error. |
| `TestRoutes_BadPrefix` | `tests/routes_test.go` | Prefix without leading `/` or with trailing `/` rejected. |
| `TestRoutes_Duplicate` | `tests/routes_test.go` | Conflicting duplicate method and path rejected with descriptive error. |
| `TestRoutes_PublicCreate` | `tests/mount_public_test.go` | App-declared public Create action mounted as `AccessPublic`. |
| `TestRoutes_PublicReadStillWorks` | `tests/mount_public_test.go` | App-declared public Read action mounted as `AccessPublic`. |
| `TestRoutes_NoPublicByDefault` | `tests/mount_public_test.go` | Routes remain guarded by default when not listed in public slice. |
| `TestMount_CrudWiringErrorStopsMount` | `tests/mount_public_test.go` | Module wiring failure (`crud.ErrNilFunction`) aborts `rpc.Mount`. |
| `TestMethodOf_PerAction` | `tests/rest_test.go` | `MethodOf` maps `Create`→POST, `Read`→GET, `Update`→PATCH, `Delete`→DELETE. |
| `TestRequest_PerAction` | `tests/rest_test.go` | Single entity generates POST, GET, PATCH, DELETE on `/api/<module>/<entity>`. |
| `TestRPCGet_ArgumentsTravelInTheQuery` | `tests/rest_test.go` | GET requests encode arguments in query parameters with no body and no Content-Type. |
| `TestRPCGet_FilterTravelsAsPrefixedKeys` | `tests/rest_test.go` | Typed filter fields travel as `filter.<field>` query keys. |
| `TestRPCResponses_AreNoStore` | `tests/rest_test.go` | Both GET and POST responses carry `Cache-Control: no-store`. |
| `TestRPCMutations_RequireJSON` | `tests/rest_test.go` | Mutations require `Content-Type: application/json` (415 otherwise); GET requires none (200). |
| `TestRPCUpdate_UsesPatchWithBody` | `tests/rest_test.go` | Update sends HTTP PATCH with JSON body containing ids, fields, and record. |
| `TestRPCDelete_UsesDeleteWithBody` | `tests/rest_test.go` | Delete sends HTTP DELETE with JSON body containing ids. |
| `TestRPCCallKeyed_RefusesARead` | `tests/rest_test.go` | `CallKeyed` refuses `model.Read` client-side with Status 0 error. |
| `TestRPCInvalidOperation` | `tests/rest_test.go` | Invalid `router.Operation` fails client-side without sending HTTP request. |
