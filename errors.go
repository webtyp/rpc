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

// The statuses an error kind travels as; WriteError writes them and Error.Kind
// reads them back, so the two directions cannot drift apart.
const (
	statusInvalid  = 400
	statusNotFound = 404
	statusConflict = 409
	statusInternal = 500
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
func (e *Error) Kind() model.ErrorKind {
	switch e.Status {
	case statusInvalid:
		return model.ErrorInvalid
	case statusNotFound:
		return model.ErrorNotFound
	case statusConflict:
		return model.ErrorConflict
	}
	return model.ErrorInternal
}

// FailedField lets model.FieldOf name the field the server said failed.
func (e *Error) FailedField() string { return e.Field }

type problem struct {
	Status int64
	Title  string
	Detail string
	Field  string
}

func (p *problem) IsNil() bool {
	return p == nil
}

func (p *problem) EncodeFields(w model.FieldWriter) {
	w.Int("status", p.Status)
	w.String("title", p.Title)
	if p.Detail != "" {
		w.String("detail", p.Detail)
	}
	if p.Field != "" {
		w.String("field", p.Field)
	}
}

func (p *problem) DecodeFields(r model.FieldReader) {
	if val, ok := r.Int("status"); ok {
		p.Status = val
	}
	if val, ok := r.String("title"); ok {
		p.Title = val
	}
	if val, ok := r.String("detail"); ok {
		p.Detail = val
	}
	if val, ok := r.String("field"); ok {
		p.Field = val
	}
}

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
	var status int64 = statusInternal
	detail := err.Error()

	switch kind {
	case model.ErrorInvalid:
		status = statusInvalid
	case model.ErrorNotFound:
		status = statusNotFound
	case model.ErrorConflict:
		status = statusConflict
	default:
		status = statusInternal
		detail = msgInternal
		ctx.SetValue(ctxKeyInternal, err.Error())
	}

	p := problem{
		Status: status,
		Title:  kind.String(),
		Detail: detail,
		Field:  model.FieldOf(err),
	}

	ctx.SetHeader(contentTypeName, contentTypeProblem)
	ctx.WriteStatus(int(status))
	ctx.Encode(&p)
}
