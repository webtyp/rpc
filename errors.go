package rpc

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/router"
)

const (
	contentTypeProblem = "application/problem+json"
	msgInternal        = "internal error"
	ctxKeyInternal     = "rpc.internal"
)

// Error is what Call reports when the server answers outside 2xx, or when the request never
// got an answer (Status 0). Callers that must tell "refused" (4xx) from "try later" (0, 5xx)
// assert it: if e, ok := err.(*rpc.Error); ok { ... }.
type Error struct {
	Status int    // HTTP status; 0 = network failure, no answer
	Body   string // response body as text (the handler's message), or the network error text
	Detail string // the problem's detail, when the server sent one
	Field  string // the field that failed, "" when none
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("rpc: no response: %s", e.Body)
	}
	if e.Detail != "" {
		return fmt.Sprintf("rpc: %d %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("rpc: %d %s", e.Status, e.Body)
}

// Kind lets model.KindOf answer in the browser what it answers on the server: the
// status the server chose tells the kind (no answer at all is internal).
func (e *Error) Kind() model.ErrorKind { return router.KindOfStatus(e.Status) }

// FailedField lets model.FieldOf name the field the server said failed.
func (e *Error) FailedField() string { return e.Field }

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
func WriteError(ctx router.Context, err error) {
	kind := model.KindOf(err)
	status := router.StatusOf(kind)
	detail := err.Error()
	if kind == model.ErrorInternal {
		detail = msgInternal
		ctx.SetValue(ctxKeyInternal, err.Error())
	}

	p := router.Problem{
		Status: int64(status),
		Title:  kind.String(),
		Detail: detail,
		Field:  model.FieldOf(err),
	}

	ctx.SetHeader(contentTypeName, contentTypeProblem)
	ctx.WriteStatus(status)
	ctx.Encode(&p)
}
