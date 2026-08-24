package tool

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/agent"
	"ratchet/internal/anchor"
	"ratchet/internal/edit"
)

// call is one tool call with its arguments.
func call(name string, args map[string]any) agent.ToolCall {
	return agent.ToolCall{Name: name, Args: args}
}

// patchText renders a one-line replacement in the notation a read is written in,
// which is what the edit tool takes.
func patchText(text, path string, line int, old, new string) string {
	return fmt.Sprintf("[%s#%s]\nPUT %d.=%d:\n-%s\n+%s", path, anchor.Tag(text), line, line, old, new)
}

// TestAReadThenAnEditFinishesTheWork is the path an iteration takes: read a file,
// cite that read in an edit, and the file on disk holds the change.
func TestAReadThenAnEditFinishesTheWork(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	_, s := write(g, t, "a.ts", text)
	tools := NewTools(s)

	got, err := tools.Execute(t.Context(), call("read", map[string]any{"path": "a.ts"}))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Text).To(ContainSubstring("2:two"))
	g.Expect(got.Stop).To(Equal(agent.StopNone))

	got, err = tools.Execute(t.Context(), call("edit", map[string]any{
		"patch": patchText(text, "a.ts", 2, "two", "2"),
	}))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Text).To(HavePrefix("applied."))
	g.Expect(got.Widened).To(BeEmpty())
}

// TestARefusalReachesTheModelUnedited is what the loop is a pipe for.
//
// The applier's refusal text is measured work: which of its causes a model can
// recover from was counted, and the wording is the part that was measured. A
// refusal is the tool's result and not an error, because the model gets another
// turn and the refusal is what it is answering.
func TestARefusalReachesTheModelUnedited(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	_, s := write(g, t, "a.ts", text)
	tools := NewTools(s)

	// No read, so the edit has no provenance and the applier refuses.
	got, err := tools.Execute(t.Context(), call("edit", map[string]any{
		"patch": patchText(text, "a.ts", 2, "two", "2"),
	}))
	g.Expect(err).NotTo(HaveOccurred(), "a refusal is a turn, not a failure")
	g.Expect(got.Text).NotTo(BeEmpty())
	g.Expect(got.Stop).To(Equal(agent.StopNone))

	// The same refusal, straight from the applier, is what the model was shown.
	_, direct := s.Preview(t.Context(), put(text, "a.ts", 2, "two", "2"), edit.Options{})
	g.Expect(direct).To(HaveOccurred())
	g.Expect(got.Text).To(Equal(direct.Error()), "the wording is the measured part and is passed through")
}

// TestTheTerminalVerbsAreTheOnlyWayToStop covers both, and that each carries what
// it said. A done is a claim rather than a decision, so what was claimed has to
// survive the call.
func TestTheTerminalVerbsAreTheOnlyWayToStop(t *testing.T) {
	cases := []struct {
		name string
		call agent.ToolCall
		want agent.Stop
		said string
	}{
		{
			name: "done",
			call: call("done", map[string]any{"summary": "renamed the field"}),
			want: agent.StopDone,
			said: "renamed the field",
		},
		{
			name: "blocked",
			call: call("blocked", map[string]any{"reason": "the file is not in this iteration"}),
			want: agent.StopBlocked,
			said: "the file is not in this iteration",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, s := write(g, t, "a.ts", "one\n")
			got, err := NewTools(s).Execute(t.Context(), c.call)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Stop).To(Equal(c.want))
			g.Expect(got.Said).To(Equal(c.said))
		})
	}
}

// TestASpellingTheSchemaDidNotAdvertiseIsTakenAndNamed is the strict-advertise,
// permissive-accept split. Refusing a call whose intent is not in doubt throws
// away a turn; taking it quietly teaches the wrong spelling.
func TestASpellingTheSchemaDidNotAdvertiseIsTakenAndNamed(t *testing.T) {
	cases := []struct {
		name        string
		args        map[string]any
		wantWidened bool
		wantErr     string
	}{
		{
			name: "the advertised spelling",
			args: map[string]any{"path": "a.ts"},
		},
		{
			name:        "file_path, which other tools call it",
			args:        map[string]any{"file_path": "a.ts"},
			wantWidened: true,
		},
		{
			name:        "a single-element array where a string was declared",
			args:        map[string]any{"path": []any{"a.ts"}},
			wantWidened: false,
		},
		{
			name:    "two values, which is a batch and has no form",
			args:    map[string]any{"path": []any{"a.ts", "b.ts"}},
			wantErr: "2 values where one was declared",
		},
		{
			name:    "no path at all",
			args:    map[string]any{"file": nil},
			wantErr: "where a string was declared",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, s := write(g, t, "a.ts", "one\n")
			got, err := NewTools(s).Execute(t.Context(), call("read", c.args))
			if c.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(c.wantErr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got.Text).To(ContainSubstring("1:one"))
			if c.wantWidened {
				g.Expect(got.Widened).NotTo(BeEmpty(), "the model has to be told which spelling to use")
				return
			}
			g.Expect(got.Widened).To(BeEmpty())
		})
	}
}

// TestAToolNobodyOfferedIsNotATurn separates a call that cannot be run from one
// that ran and was refused. Another turn does not fix a name that does not exist,
// so it is an error rather than a result.
func TestAToolNobodyOfferedIsNotATurn(t *testing.T) {
	g := NewWithT(t)
	_, s := write(g, t, "a.ts", "one\n")
	_, err := NewTools(s).Execute(t.Context(), call("write", map[string]any{"path": "a.ts"}))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("read, edit, done and blocked"))
}

// TestTheAdvertisedSchemasAreStrict guards what the model is shown: four tools,
// each requiring exactly the argument its description names.
func TestTheAdvertisedSchemasAreStrict(t *testing.T) {
	g := NewWithT(t)
	_, s := write(g, t, "a.ts", "one\n")
	advertised := NewTools(s).Tools()

	g.Expect(advertised).To(HaveLen(4))
	wanted := map[string]string{
		"read":    "path",
		"edit":    "patch",
		"done":    "summary",
		"blocked": "reason",
	}
	for _, tool := range advertised {
		g.Expect(tool.Description).NotTo(BeEmpty(), "a tool with no description is a guess")
		g.Expect(tool.Schema).To(HaveKeyWithValue("required", []string{wanted[tool.Name]}))
	}
}

// TestTheToolListReadsAsProse keeps the message a model is shown in plain
// English, since the tool names are the one part of a refusal it can act on.
func TestTheToolListReadsAsProse(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{name: "none", in: nil, want: ""},
		{name: "one", in: []string{"read"}, want: "read"},
		{name: "two", in: []string{"read", "edit"}, want: "read and edit"},
		{name: "the four", in: Names(), want: "read, edit, done and blocked"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(list(c.in)).To(Equal(c.want))
		})
	}
}
