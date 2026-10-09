package rpc

import (
	"webtyp.com/fetch"
	"webtyp.com/json"
	"webtyp.com/model"
	"webtyp.com/router"
)

// Caller sends operations over HTTP. It implements router.Caller.
type Caller struct {
	origin string
}

// NewCaller returns a Caller for origin ("" = the page's own origin). Uses DefaultPrefix.
func NewCaller(origin string) *Caller {
	return &Caller{origin: origin}
}

func (c *Caller) Dispatch(op string, args model.Encodable) {
	c.Call(op, args, nil, nil)
}

func (c *Caller) Call(op string, args model.Encodable, into model.Decodable, done func(err error)) {
	var body []byte
	if args == nil || args.IsNil() {
		body = []byte("{}")
	} else if err := json.Encode(args, &body); err != nil {
		if done != nil {
			done(&Error{Status: 0, Body: err.Error()})
		}
		return
	}
	c.send(op, "", body, into, done)
}

// CallKeyed is Call with the request header router.HeaderIdempotencyKey set to key, so a server
// running the idempotency middleware answers a repeated send once. key == "" → done receives an
// *Error with Status 0 and Body "rpc: idempotency key is required" and nothing is sent.
func (c *Caller) CallKeyed(op, key string, body []byte, into model.Decodable, done func(err error)) {
	if key == "" {
		if done != nil {
			done(&Error{Status: 0, Body: "rpc: idempotency key is required"})
		}
		return
	}
	if len(body) == 0 {
		body = []byte("{}")
	}
	c.send(op, key, body, into, done)
}

func (c *Caller) send(op, key string, body []byte, into model.Decodable, done func(err error)) {
	dotIdx := -1
	for i := 0; i < len(op); i++ {
		if op[i] == '.' {
			if dotIdx != -1 {
				dotIdx = -1 // more than one dot
				break
			}
			dotIdx = i
		}
	}
	if dotIdx == -1 {
		if done != nil {
			done(&Error{Status: 0, Body: "rpc: operation name must be <module>.<op>"})
		}
		return
	}

	module := op[:dotIdx]
	opName := op[dotIdx+1:]
	url := c.origin + DefaultPrefix + slash + module + slash + opName

	req := fetch.Post(url).ContentTypeJSON()
	if key != "" {
		req.Header(router.HeaderIdempotencyKey, key)
	}
	req.Body(body).Send(func(resp *fetch.Response, err error) {
		if done == nil {
			return
		}
		if err != nil {
			done(&Error{Status: 0, Body: err.Error()})
			return
		}
		if resp.Status < 200 || resp.Status > 299 {
			done(&Error{Status: resp.Status, Body: resp.Text()})
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
