package agent

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestOneCallPerTurn is the rule the schemas have no form for breaking.
//
// Two calls in one reply is not a queue. A partly applied pair is not a state
// this system can represent, so the second call would have to be dropped or run
// against a file the first one changed, and neither is done quietly. The message
// names every call, including a repeat, because two calls to the same tool is the
// common shape of the mistake.
func TestOneCallPerTurn(t *testing.T) {
	cases := []struct {
		name    string
		calls   []ToolCall
		want    string
		wantErr []string
	}{
		{
			name:  "one call",
			calls: []ToolCall{{Name: "read"}},
			want:  "read",
		},
		{
			name:    "none at all",
			calls:   nil,
			wantErr: []string{"no tool call"},
		},
		{
			name:    "two different tools",
			calls:   []ToolCall{{Name: "read"}, {Name: "edit"}},
			wantErr: []string{"2 tool calls", "read, edit"},
		},
		{
			name:    "the same tool twice",
			calls:   []ToolCall{{Name: "edit"}, {Name: "edit"}},
			wantErr: []string{"2 tool calls", "edit, edit"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := OneCall(c.calls)
			if len(c.wantErr) > 0 {
				g.Expect(err).To(HaveOccurred())
				for _, want := range c.wantErr {
					g.Expect(err.Error()).To(ContainSubstring(want))
				}
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Name).To(Equal(c.want))
		})
	}
}

// TestAStopNamesItself keeps the two terminal verbs readable in a message, since
// both are named in every failure the executor is shown.
func TestAStopNamesItself(t *testing.T) {
	cases := []struct {
		name string
		in   Stop
		want string
	}{
		{name: "none", in: StopNone, want: "none"},
		{name: "done", in: StopDone, want: "done"},
		{name: "blocked", in: StopBlocked, want: "blocked"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(c.in.String()).To(Equal(c.want))
		})
	}
}
