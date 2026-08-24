package agent

import (
	"context"

	"github.com/cockroachdb/errors"
)

// Script is a provider that replies from a list, so the loop can be tested
// without a host.
//
// A test that needs a model is a test that needs a GPU, a loaded model and a
// tolerance for a model changing its mind. The loop's shape is decided by what
// comes back, not by who sent it, so what comes back is written down here.
type Script struct {
	// Replies are returned in order, one a turn.
	Replies []Reply
	// Context is what Allocated reports.
	Context int
	// Err, when set, is returned by Stream instead of the next reply.
	Err error

	// Seen records every request in order, so a test can assert what the loop
	// sent as well as what it did with the answer.
	Seen []Request
	// Streamed records the pieces passed to the callback, in order.
	Streamed []Event

	turn int
}

// Stream returns the next scripted reply, recording the request.
//
// It delivers the reply's text as a single piece rather than token by token. A
// test that needs to see several pieces sets Replies with the content already
// split, which keeps this from inventing a tokenizer.
func (s *Script) Stream(ctx context.Context, req Request, on func(Event)) (Reply, error) {
	s.Seen = append(s.Seen, req)
	if s.Err != nil {
		return Reply{}, s.Err
	}
	if s.turn >= len(s.Replies) {
		return Reply{}, errors.Newf("the script ran out after %d turns", len(s.Replies))
	}
	reply := s.Replies[s.turn]
	s.turn++
	if on != nil && (reply.Content != "" || reply.Thinking != "") {
		e := Event{Content: reply.Content, Thinking: reply.Thinking}
		s.Streamed = append(s.Streamed, e)
		on(e)
	}
	return reply, nil
}

// Allocated reports the context the script was given, and refuses when it has
// none, which is what an unloaded model looks like.
func (s *Script) Allocated(ctx context.Context, model string) (int, error) {
	if s.Context == 0 {
		return 0, errors.Newf("%s is not loaded", model)
	}
	return s.Context, nil
}

// Turns is how many replies the script has handed out.
func (s *Script) Turns() int {
	return s.turn
}

// compile-time proof that the script satisfies the interface the loop holds.
var _ Provider = (*Script)(nil)
