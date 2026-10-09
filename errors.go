package rpc

import "webtyp.com/fmt"

// Error is what Call reports when the server answers outside 2xx, or when the request never
// got an answer (Status 0). Callers that must tell "refused" (4xx) from "try later" (0, 5xx)
// assert it: if e, ok := err.(*rpc.Error); ok { ... }.
type Error struct {
	Status int    // HTTP status; 0 = network failure, no answer
	Body   string // response body as text (the handler's message), or the network error text
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("rpc: no response: %s", e.Body)
	}
	return fmt.Sprintf("rpc: %d %s", e.Status, e.Body)
}

type rpcError string

func (e rpcError) Error() string {
	return string(e)
}
