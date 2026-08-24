package agent

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestTheScriptReplaysInOrderAndRecords is what lets the loop be tested without a
// host: replies come back in the order written, and every request is kept so a
// test can assert what was sent as well as what was done with the answer.
func TestTheScriptReplaysInOrderAndRecords(t *testing.T) {
	g := NewWithT(t)
	s := &Script{
		Context: 20480,
		Replies: []Reply{
			{Content: "first"},
			{ToolCalls: []ToolCall{{Name: "read", Args: map[string]any{"path": "a.go"}}}},
		},
	}

	one, err := s.Stream(t.Context(), Request{Model: "m", Messages: []Message{{Content: "go"}}}, nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(one.Content).To(Equal("first"))

	two, err := s.Stream(t.Context(), Request{Model: "m"}, nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(two.ToolCalls).To(HaveLen(1))

	g.Expect(s.Turns()).To(Equal(2))
	g.Expect(s.Seen).To(HaveLen(2))
	g.Expect(s.Seen[0].Messages[0].Content).To(Equal("go"))
}

// TestTheScriptRefusesToInventATurn keeps a test that runs longer than its script
// from passing on a reply nobody wrote. A silent empty reply would read as a
// model that said nothing, which is a real outcome and would be the wrong one.
func TestTheScriptRefusesToInventATurn(t *testing.T) {
	g := NewWithT(t)
	s := &Script{Context: 20480, Replies: []Reply{{Content: "only one"}}}

	_, err := s.Stream(t.Context(), Request{}, nil)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = s.Stream(t.Context(), Request{}, nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("ran out after 1 turns"))
}
