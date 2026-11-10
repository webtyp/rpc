package rpc

import (
	"webtyp.com/fetch"
	"webtyp.com/json"
	"webtyp.com/model"
	"webtyp.com/router"
)

const (
	msgKeyRequired = "rpc: idempotency key is required"
	msgKeyedRead   = "rpc: a read has no idempotency key; use Call"
	msgBadOp       = "rpc: invalid operation: "
)

// Caller sends operations over HTTP. It implements router.Caller.
type Caller struct {
	origin string
}

// NewCaller returns a Caller for origin ("" = the page's own origin). Uses DefaultPrefix.
func NewCaller(origin string) *Caller {
	return &Caller{origin: origin}
}

// Dispatch is Call without a result: fire and forget.
func (c *Caller) Dispatch(op router.Operation, args model.Encodable) {
	c.Call(op, args, nil, nil)
}

// Call sends the operation as its REST request (see requestOf) and decodes a 2xx answer into into. A Read travels in the query,
// the other actions as a JSON body. Any other outcome reaches done as an *Error.
func (c *Caller) Call(op router.Operation, args model.Encodable, into model.Decodable, done func(err error)) {
	method, path, ok := requestOf(DefaultPrefix, op)
	if !ok {
		c.fail(done, msgBadOp+op.String())
		return
	}
	if !hasBody(method) { // GET: the arguments are the query
		query := ""
		if args != nil && !args.IsNil() {
			q, err := router.EncodeQuery(args)
			if err != nil {
				c.fail(done, err.Error())
				return
			}
			query = q
		}
		c.send(method, c.origin+path, query, "", nil, into, done)
		return
	}
	var body []byte
	if args == nil || args.IsNil() {
		body = []byte("{}")
	} else if err := json.Encode(args, &body); err != nil {
		c.fail(done, err.Error())
		return
	}
	c.send(method, c.origin+path, "", "", body, into, done)
}

// CallKeyed is Call with the request header router.HeaderIdempotencyKey set to key, so a server running the idempotency middleware
// answers a repeated send once. A read has no key (it is safe to repeat): CallKeyed refuses it. key == "" → done receives an *Error
// with Status 0 and nothing is sent. body is the operation's args already encoded as JSON (as a queue stores them); empty sends "{}".
func (c *Caller) CallKeyed(op router.Operation, key string, body []byte, into model.Decodable, done func(err error)) {
	method, path, ok := requestOf(DefaultPrefix, op)
	if !ok {
		c.fail(done, msgBadOp+op.String())
		return
	}
	if !hasBody(method) {
		c.fail(done, msgKeyedRead)
		return
	}
	if key == "" {
		c.fail(done, msgKeyRequired)
		return
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	c.send(method, c.origin+path, "", key, body, into, done)
}

func (c *Caller) fail(done func(err error), text string) {
	if done != nil {
		done(&Error{Status: 0, Body: text})
	}
}

func (c *Caller) send(method, url, query, key string, body []byte, into model.Decodable, done func(err error)) {
	if query != "" {
		url += "?" + query
	}
	var req *fetch.Request
	switch method {
	case methodGet:
		req = fetch.Get(url)
	case methodPost:
		req = fetch.Post(url)
	case methodPatch:
		req = fetch.Patch(url)
	default:
		req = fetch.Delete(url)
	}
	if key != "" {
		req.Header(router.HeaderIdempotencyKey, key)
	}
	if hasBody(method) {
		req = req.ContentTypeJSON().Body(body)
	}
	req.Send(func(resp *fetch.Response, err error) {
		if done == nil {
			return
		}
		if err != nil {
			done(&Error{Status: 0, Body: err.Error()})
			return
		}
		if resp.Status < 200 || resp.Status > 299 {
			errResp := &Error{Status: resp.Status, Body: resp.Text()}
			if len(errResp.Body) > 0 && errResp.Body[0] == '{' {
				var p router.Problem
				if decodeErr := json.Decode(errResp.Body, &p); decodeErr == nil {
					errResp.Detail = p.Detail
					errResp.Field = p.Field
				}
			}
			done(errResp)
			return
		}
		if into != nil {
			if respText := resp.Text(); len(respText) > 0 {
				done(json.Decode(respText, into))
				return
			}
		}
		done(nil)
	})
}

var _ router.Caller = (*Caller)(nil)
