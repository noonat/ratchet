package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
)

// StopReasonLength is the host's word for a reply that reached the output cap.
const StopReasonLength = "length"

// DefaultMaxTurns bounds one iteration.
//
// Not a retry budget: there is one attempt and a failure ends it. This stops a
// model that never reaches a terminal verb from running until something else
// does, which nothing else here would.
const DefaultMaxTurns = 40

// Iteration is the work one run of the loop is given.
//
// The prompt arrives as text because the seat owns it. The loop assembles a
// thread and drives it; what to say belongs to whoever knows which seat this is.
type Iteration struct {
	// Model is the model to run.
	Model string
	// System is the seat's standing instruction.
	System string
	// Task is the iteration's own text, as the model is given it.
	Task string
	// NumCtx is the context to ask for, and NeedContext the least that will do.
	NumCtx int
	// NeedContext refuses the run when the host gave the model less, with zero
	// skipping the check.
	NeedContext int
	// Predict caps a reply, with zero leaving it to the host.
	Predict int
	// Think is the reasoning setting, empty to leave the field off.
	Think string
	// MaxTurns bounds the run, defaulting to DefaultMaxTurns.
	MaxTurns int
}

// Report is what one run of the loop produced.
type Report struct {
	// Stop is the terminal verb the model reached.
	Stop Stop
	// Said is what that verb carried.
	Said string
	// Turns is how many replies the model gave.
	Turns int
	// Thread is the conversation, for the journal.
	Thread []Message
}

// Run drives one iteration to a terminal verb.
//
// One attempt. A tool refusal is a turn and the model answers it; anything the
// loop cannot make a turn out of ends the run and says why.
func Run(ctx context.Context, p Provider, d Dispatcher, it Iteration) (Report, error) {
	thread := []Message{{Role: RoleSystem, Content: it.System}, {Role: RoleUser, Content: it.Task}}
	max := it.MaxTurns
	if max <= 0 {
		max = DefaultMaxTurns
	}

	for turn := 1; turn <= max; turn++ {
		req := Request{
			Model:    it.Model,
			Messages: thread,
			Tools:    d.Tools(),
			NumCtx:   it.NumCtx,
			Predict:  it.Predict,
			Think:    it.Think,
		}
		reply, err := p.Stream(ctx, req, nil)
		if err != nil {
			return Report{Turns: turn, Thread: thread}, errors.Wrapf(err, "turn %d", turn)
		}
		thread = append(thread, Message{
			Role:      RoleAssistant,
			Content:   reply.Content,
			ToolCalls: reply.ToolCalls,
			Thinking:  reply.Thinking,
			Done:      reply.Done,
		})

		// After the first turn, because the first request is what loads the model
		// and a host reports what it gave one only once it holds it. One turn is
		// the cost of asking a question that has no answer before it.
		if turn == 1 && it.NeedContext > 0 {
			if err := RequireContext(ctx, p, it.Model, it.NeedContext); err != nil {
				return Report{Turns: turn, Thread: thread}, err
			}
		}

		call, err := OneCall(reply.ToolCalls)
		if err != nil {
			// A reply that stopped at the cap carried no call because it never got
			// that far. Reporting a missing call blames the model's tool-call
			// format for a truncation, which sends the reader looking in the wrong
			// place.
			if reply.Done == StopReasonLength {
				return Report{Turns: turn, Thread: thread}, errors.Newf(
					"turn %d: the reply reached the %d token cap before calling anything: %s",
					turn,
					it.Predict,
					terminalVerbs(),
				)
			}
			return Report{Turns: turn, Thread: thread}, errors.Wrapf(err, "turn %d: %s", turn, terminalVerbs())
		}

		out, err := d.Execute(ctx, call)
		if err != nil {
			return Report{Turns: turn, Thread: thread}, errors.Wrapf(err, "turn %d: %s", turn, terminalVerbs())
		}
		if out.Stop != StopNone {
			return Report{Stop: out.Stop, Said: out.Said, Turns: turn, Thread: thread}, nil
		}
		thread = append(thread, Message{
			Role:     RoleTool,
			ToolName: call.Name,
			Content:  result(out, turn, max),
		})
	}
	return Report{Turns: max, Thread: thread}, errors.Newf("%d turns without stopping: %s", max, terminalVerbs())
}

// result is the text a tool's outcome goes back as.
//
// The refusal itself is untouched, because its wording is measured work. What the
// loop adds is its own: which spelling was accepted, and how little room is left.
func result(out Outcome, turn, max int) string {
	var b strings.Builder
	b.WriteString(out.Text)
	for _, w := range out.Widened {
		fmt.Fprintf(&b, "\n\n(%s. use the advertised name.)", w)
	}
	// Keyed on the turns still to come, not on the turn that just ran. A warning
	// written on the final turn is appended after the loop has ended and no turn
	// follows it, so the model never reads it.
	switch left := max - turn; {
	case left == 1:
		fmt.Fprintf(&b, "\n\n(this is the last turn. %s)", terminalVerbs())
	case left > 1 && left <= turnsLeftWarning:
		fmt.Fprintf(&b, "\n\n(%d turns left. %s)", left, terminalVerbs())
	}
	return b.String()
}

// turnsLeftWarning is how close to the bound the loop starts saying so, giving a
// model room to stop on purpose rather than being cut off.
const turnsLeftWarning = 3

// terminalVerbs names the two ways to stop. Every failure the loop writes carries
// it, because a model that cannot see how to stop keeps trying the thing that
// failed.
func terminalVerbs() string {
	return "stop with done or blocked"
}
