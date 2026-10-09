package rpc

import (
	"webtyp.com/fetch"
	"webtyp.com/json"
	"webtyp.com/model"
	"webtyp.com/router"
)

type clr struct {
	origin string
}

// NewCaller returns a router.Caller that sends each Call as POST <origin><prefix>/<module>/<op>
// with a JSON body. origin "" means the page's own origin (relative URLs). Uses DefaultPrefix.
func NewCaller(origin string) router.Caller {
	return &clr{origin: origin}
}

func (c *clr) Dispatch(op string, args model.Encodable) {
	c.Call(op, args, nil, nil)
}

func (c *clr) Call(op string, args model.Encodable, into model.Decodable, done func(err error)) {
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

	var body []byte
	if args == nil || args.IsNil() {
		body = []byte("{}")
	} else {
		json.Encode(args, &body)
	}

	fetch.Post(url).ContentTypeJSON().Body(body).Send(func(resp *fetch.Response, err error) {
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
