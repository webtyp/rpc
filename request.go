package rpc

import (
	"webtyp.com/model"
	"webtyp.com/router"
)

const (
	methodGet    = "GET"
	methodPost   = "POST"
	methodPatch  = "PATCH"
	methodDelete = "DELETE"

	headerCacheControl = "Cache-Control"
	cacheNoStore       = "no-store"
)

// MethodOf is the HTTP method an action uses: POST to create, GET to read, PATCH to update, DELETE to delete; "" for anything else.
// appkit uses it to find the route of a public action; the server and the client use it through requestOf.
//
//	rpc.MethodOf(model.Read) // "GET"
func MethodOf(a model.Action) string {
	switch a {
	case model.Create:
		return methodPost
	case model.Read:
		return methodGet
	case model.Update:
		return methodPatch
	case model.Delete:
		return methodDelete
	}
	return ""
}

// requestOf is the HTTP request an operation uses: the method is the action, the path is the resource.
// The server registers it and the client sends it, so the two cannot drift apart. An invalid operation has none.
//
//	method, path, ok := requestOf("/api", router.Operation{Module: "agenda", Entity: "patient", Action: model.Read}) // GET /api/agenda/patient
func requestOf(prefix string, op router.Operation) (method, path string, ok bool) {
	if !op.Valid() {
		return "", "", false
	}
	return MethodOf(op.Action), prefix + slash + op.Module + slash + op.Entity, true
}

// hasBody reports whether requests of this method carry a JSON body.
func hasBody(method string) bool { return method != methodGet }
