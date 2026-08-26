package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

// stub dispatches from a table, so a loop test says what the tools did without
// needing tools.
type stub struct {
	out  []Outcome
	err  error
	seen []ToolCall
	turn int
}

func (s *stub) Tools() []Tool {
	return []Tool{{Name: "read"}, {Name: "done"}}
}

func (s *stub) Execute(ctx context.Context, call ToolCall) (Outcome, error) {
	s.seen = append(s.seen, call)
	if s.err != nil {
		return Outcome{}, s.err
	}
	out := s.out[s.turn]
	s.turn++
	return out, nil
}

// asking returns a reply that calls one tool.
func asking(name string) Reply {
	return Reply{ToolCalls: []ToolCall{{Name: name}}}
}

// TestTheLoopRunsUntilATerminalVerb is the shape of one iteration: the model is
// given the work, calls a tool, is handed the result, and stops on purpose.
func TestTheLoopRunsUntilATerminalVerb(t *testing.T) {
	g := NewWithT(t)
	p := &Script{
		Context: 20480,
		Replies: []Reply{asking("read"), asking("read"), asking("done")},
	}
	d := &stub{out: []Outcome{
		{Text: "1:one"},
		{Text: "1:one"},
		{Text: "recorded", Stop: StopDone, Said: "renamed it"},
	}}

	got, err := Run(t.Context(), p, d, Iteration{
		Model:       "m",
		System:      "you are an executor",
		Task:        "rename the field",
		NeedContext: 20480,
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Stop).To(Equal(StopDone))
	g.Expect(got.Said).To(Equal("renamed it"))
	g.Expect(got.Turns).To(Equal(3))

	// system, task, then an assistant and a tool message a turn, and the last
	// assistant reply with no tool result after it because the run ended.
	g.Expect(got.Thread).To(HaveLen(7))
	g.Expect(got.Thread[0].Role).To(Equal(RoleSystem))
	g.Expect(got.Thread[1].Content).To(Equal("rename the field"))
	g.Expect(got.Thread[3].Role).To(Equal(RoleTool))
	g.Expect(got.Thread[3].Content).To(Equal("1:one"))
}

// TestARefusalIsATurnAndReachesTheModelWhole separates the two failure kinds. A
// refused tool is answered by another turn, and its wording is not touched.
func TestARefusalIsATurnAndReachesTheModelWhole(t *testing.T) {
	g := NewWithT(t)
	const refusal = "no read in this session served a.ts"
	p := &Script{
		Context: 20480,
		Replies: []Reply{
			asking("read"),
			asking("done"),
		},
	}
	d := &stub{
		out: []Outcome{
			{Text: refusal},
			{Stop: StopDone},
		},
	}

	got, err := Run(t.Context(), p, d, Iteration{Model: "m"})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Turns).To(Equal(2))
	g.Expect(got.Thread[3].Content).
		To(Equal(refusal), "the refusal is measured work and the loop is a pipe for it")
}

// TestAProtocolFailureEndsTheRunAndNamesHowToStop covers what another turn cannot
// fix. Each message names both verbs, because a model that cannot see how to stop
// keeps trying the thing that failed.
func TestAProtocolFailureEndsTheRunAndNamesHowToStop(t *testing.T) {
	cases := []struct {
		name    string
		replies []Reply
		err     error
		want    string
	}{
		{
			name: "two calls in one reply",
			replies: []Reply{
				{
					ToolCalls: []ToolCall{
						{Name: "read"},
						{Name: "edit"},
					},
				},
			},
			want: "2 tool calls",
		},
		{
			name: "no call at all",
			replies: []Reply{
				{Content: "I think the answer is 4"},
			},
			want: "no tool call",
		},
		{
			name:    "a call the dispatcher cannot run",
			replies: []Reply{asking("write")},
			err:     errors.New("no tool named write"),
			want:    "no tool named write",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := &Script{Context: 20480, Replies: c.replies}
			d := &stub{err: c.err, out: []Outcome{{}}}

			got, err := Run(t.Context(), p, d, Iteration{Model: "m"})

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.want))
			g.Expect(err.Error()).
				To(ContainSubstring("done or blocked"), "every failure names both terminal verbs")
			g.Expect(got.Stop).To(Equal(StopNone))
			g.Expect(got.Thread).NotTo(BeEmpty(), "the thread survives a failure, for the journal")
		})
	}
}

// TestTheLoopStopsRatherThanRunningForever bounds a model that never reaches a
// terminal verb. The bound is not a retry budget: there is one attempt, and this
// is the only other thing that would end it.
func TestTheLoopStopsRatherThanRunningForever(t *testing.T) {
	g := NewWithT(t)
	const turns = 4
	replies := make([]Reply, turns)
	outs := make([]Outcome, turns)
	for i := range replies {
		replies[i] = asking("read")
		outs[i] = Outcome{Text: "1:one"}
	}
	p := &Script{Context: 20480, Replies: replies}
	d := &stub{out: outs}

	got, err := Run(t.Context(), p, d, Iteration{Model: "m", MaxTurns: turns})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("4 turns without stopping"))
	g.Expect(err.Error()).To(ContainSubstring("done or blocked"))
	g.Expect(got.Turns).To(Equal(turns))

	// The last few results warn, so a model can stop on purpose instead of being
	// cut off mid-thought.
	// The warning has to arrive as the input to a turn the model can still act
	// on. The entry appended after the final turn is never sent anywhere.
	delivered := got.Thread[len(got.Thread)-3].Content
	g.Expect(delivered).To(ContainSubstring("this is the last turn"))
	g.Expect(delivered).To(ContainSubstring("done or blocked"))
	g.Expect(got.Thread[len(got.Thread)-5].Content).
		To(ContainSubstring("2 turns left"), "the warning counts down before it")
}

// TestAWidenedSpellingIsNamedInTheResult tells the model which name to use, once,
// beside the result rather than instead of it.
func TestAWidenedSpellingIsNamedInTheResult(t *testing.T) {
	g := NewWithT(t)
	p := &Script{Context: 20480, Replies: []Reply{asking("read"), asking("done")}}
	d := &stub{out: []Outcome{
		{Text: "1:one", Widened: []string{"path was given as file_path"}},
		{Stop: StopDone},
	}}

	got, err := Run(t.Context(), p, d, Iteration{Model: "m"})

	g.Expect(err).NotTo(HaveOccurred())
	result := got.Thread[3].Content
	g.Expect(result).To(HavePrefix("1:one"), "the result comes first")
	g.Expect(result).To(ContainSubstring("file_path"))
	g.Expect(strings.Count(result, "advertised name")).To(Equal(1))
}

// cold answers as a host does before it holds a model: the allocation cannot be
// read until a request has loaded one.
type cold struct {
	*Script
	loaded bool
}

func (c *cold) Stream(ctx context.Context, req Request, on func(Event)) (Reply, error) {
	c.loaded = true
	return c.Script.Stream(ctx, req, on)
}

func (c *cold) Allocated(ctx context.Context, model string) (int, error) {
	if !c.loaded {
		return 0, errors.Wrapf(ErrNotLoaded, "%s", model)
	}
	return c.Script.Allocated(ctx, model)
}

// TestAColdModelIsNotAShortfall is the defect this fixes.
//
// The check ran before the first turn, and a host reports what it gave a model
// only once it holds one. So a check meant to refuse an under-provisioned model
// refused every model that had not been loaded yet, and a live run had to be
// warmed by hand. The first request is what loads it, so the question is asked
// after that request and not before.
func TestAColdModelIsNotAShortfall(t *testing.T) {
	g := NewWithT(t)
	p := &cold{Script: &Script{
		Context: 20480,
		Replies: []Reply{asking("done")},
	}}
	d := &stub{
		out: []Outcome{
			{Stop: StopDone},
		},
	}

	got, err := Run(t.Context(), p, d, Iteration{Model: "m", NeedContext: 20480})

	g.Expect(err).
		NotTo(HaveOccurred(), "a cold model is loaded by the first turn, not refused before it")
	g.Expect(got.Stop).To(Equal(StopDone))
	g.Expect(p.Turns()).To(Equal(1))
}

// TestAShortfallStillStopsTheRun keeps the check doing its job. It now stops on
// the second turn rather than the first, which costs one turn and answers a
// question that has no answer before it.
func TestAShortfallStillStopsTheRun(t *testing.T) {
	g := NewWithT(t)
	p := &cold{Script: &Script{
		Context: 4096,
		Replies: []Reply{
			asking("read"),
			asking("done"),
		},
	}}
	d := &stub{
		out: []Outcome{
			{Text: "1:one"},
			{Stop: StopDone},
		},
	}

	got, err := Run(t.Context(), p, d, Iteration{Model: "m", NeedContext: 20480})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("needs 20480"))
	g.Expect(got.Turns).To(Equal(1), "it stops after the turn that loaded the model")
	g.Expect(p.Turns()).To(Equal(1), "and does not spend a second one")
}

// TestAModelThatAnswersButIsNotHeldIsReported covers the odd case: a reply
// arrived and the host still does not list the model. That is not a shortfall
// and saying so is better than reporting a context of zero.
func TestAModelThatAnswersButIsNotHeldIsReported(t *testing.T) {
	g := NewWithT(t)
	p := &cold{Script: &Script{
		Context: 20480,
		Replies: []Reply{
			asking("done"),
		},
	}}
	p.loaded = false
	// Stream sets loaded, so force the host to keep denying it.
	stubborn := &neverHeld{cold: p}

	d := &stub{
		out: []Outcome{
			{Stop: StopDone},
		},
	}
	_, err := Run(t.Context(), stubborn, d, Iteration{Model: "m", NeedContext: 20480})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("answered but the host is not holding it"))
}

// neverHeld answers turns and never admits to holding the model.
type neverHeld struct{ *cold }

func (n *neverHeld) Allocated(ctx context.Context, model string) (int, error) {
	return 0, errors.Wrapf(ErrNotLoaded, "%s", model)
}

// TestATruncatedReplyIsNotAMissingToolCall separates two failures that arrive
// looking identical.
//
// A reasoning model that reaches the output cap returns empty text and no tool
// calls. Reported as a missing call, that blames the model's tool-call format for
// a truncation and sends the reader to the wrong place. The stop reason is the
// only thing that says which happened, and the loop dropped it.
func TestATruncatedReplyIsNotAMissingToolCall(t *testing.T) {
	cases := []struct {
		name  string
		reply Reply
		want  string
	}{
		{
			name:  "spent the whole cap reasoning",
			reply: Reply{Thinking: "step one, step two", Done: StopReasonLength},
			want:  "reached the 4096 token cap",
		},
		{
			name:  "answered in prose and called nothing",
			reply: Reply{Content: "I think the answer is 4", Done: "stop"},
			want:  "no tool call",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p := &Script{
				Context: 20480,
				Replies: []Reply{c.reply},
			}
			d := &stub{
				out: []Outcome{{}},
			}
			got, err := Run(t.Context(), p, d, Iteration{Model: "m", Predict: 4096})

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.want))
			g.Expect(err.Error()).To(ContainSubstring("done or blocked"))
			g.Expect(got.Thread).NotTo(BeEmpty())
		})
	}
}

// TestTheThreadKeepsWhatArrived guards the journal. A reply's reasoning and its
// stop reason are what explain an empty turn later, and the thread is the only
// place they are kept.
func TestTheThreadKeepsWhatArrived(t *testing.T) {
	g := NewWithT(t)
	p := &Script{Context: 20480, Replies: []Reply{
		{Content: "", Thinking: "working through it", Done: "stop",
			ToolCalls: []ToolCall{{Name: "done"}}},
	}}

	d := &stub{
		out: []Outcome{
			{Stop: StopDone},
		},
	}
	got, err := Run(t.Context(), p, d, Iteration{Model: "m"})

	g.Expect(err).NotTo(HaveOccurred())
	assistant := got.Thread[2]
	g.Expect(assistant.Role).To(Equal(RoleAssistant))
	g.Expect(assistant.Thinking).To(Equal("working through it"))
	g.Expect(assistant.Done).To(Equal("stop"))
}
