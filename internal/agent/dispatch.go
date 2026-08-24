package agent

import (
	"context"

	"github.com/cockroachdb/errors"
)

// Dispatcher is the tool surface a seat gives the loop, so the loop never learns
// which seat it is running.
type Dispatcher interface {
	// Tools are the calls to advertise, with strict schemas.
	Tools() []Tool
	// Execute runs one call and returns what the model is shown.
	//
	// A refused call is an Outcome carrying the refusal, because the model gets
	// another turn. An error means another turn would not help.
	Execute(ctx context.Context, call ToolCall) (Outcome, error)
}

// Stop is why an iteration ended.
type Stop int

const (
	// StopNone means the iteration continues.
	StopNone Stop = iota
	// StopDone means the model claimed the work is finished. A claim, not a
	// decision.
	StopDone
	// StopBlocked means the model stopped on purpose and said why.
	StopBlocked
)

// String names a stop for a message.
func (s Stop) String() string {
	switch s {
	case StopDone:
		return "done"
	case StopBlocked:
		return "blocked"
	default:
		return "none"
	}
}

// Outcome is what one call produced.
type Outcome struct {
	// Text is what goes back to the model as the call's result.
	Text string
	// Stop is set by a terminal call.
	Stop Stop
	// Said is the summary a done carried or the reason a blocked carried.
	Said string
	// Widened names each argument taken under a spelling the schema did not
	// advertise, so the model can be told which to use next time.
	Widened []string
}

// OneCall takes the single call a reply may carry.
//
// Two calls is an error, not a queue: a half-applied pair is not a state this
// system can represent.
func OneCall(calls []ToolCall) (ToolCall, error) {
	switch len(calls) {
	case 1:
		return calls[0], nil
	case 0:
		return ToolCall{}, errors.New("the reply carried no tool call")
	default:
		names := make([]string, 0, len(calls))
		for _, c := range calls {
			names = append(names, c.Name)
		}
		return ToolCall{}, errors.Newf(
			"the reply carried %d tool calls (%s) and one turn takes one call",
			len(calls),
			joinNames(names),
		)
	}
}

// joinNames renders call names, keeping duplicates: two calls to the same tool is
// the common form of this mistake.
func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
