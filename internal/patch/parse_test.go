package patch

import (
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

func TestParseAcceptsBothForms(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		path  string
		want  Hunk
	}{
		{
			name:  "whole line",
			reply: "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 1;\n+const n = 2;",
			want: Hunk{
				Kind: KindPut,
				Line: 12,
				End:  12,
				Old:  []string{"const n = 1;"},
				New:  []string{"const n = 2;"},
			},
		},
		{
			name:  "line range",
			reply: "[a/b.ts#1A2B]\nPUT 12.=14:\n-one\n-two\n-three\n+only",
			want: Hunk{
				Kind: KindPut,
				Line: 12,
				End:  14,
				Old:  []string{"one", "two", "three"},
				New:  []string{"only"},
			},
		},
		{
			name:  "fragment",
			reply: "[a/b.ts#1A2B]\nSUB 12:\n-const\n+let",
			want: Hunk{
				Kind: KindSub,
				Line: 12,
				End:  12,
				Old:  []string{"const"},
				New:  []string{"let"},
			},
		},
		{
			name:  "content keeps its leading whitespace",
			reply: "[a/b.ts#1A2B]\nPUT 3.=3:\n-    return 1\n+    return 2",
			want: Hunk{
				Kind: KindPut,
				Line: 3,
				End:  3,
				Old:  []string{"    return 1"},
				New:  []string{"    return 2"},
			},
		},
		{
			name:  "content starting with a dash needs no escape",
			path:  "a/b.md",
			reply: "[a/b.md#1A2B]\nPUT 5.=5:\n-- item\n+- item (checked)",
			want: Hunk{
				Kind: KindPut,
				Line: 5,
				End:  5,
				Old:  []string{"- item"},
				New:  []string{"- item (checked)"},
			},
		},
		{
			name:  "content starting with a plus needs no escape",
			path:  "a/b.md",
			reply: "[a/b.md#1A2B]\nPUT 5.=5:\n-+ added\n++ added twice",
			want: Hunk{
				Kind: KindPut,
				Line: 5,
				End:  5,
				Old:  []string{"+ added"},
				New:  []string{"+ added twice"},
			},
		},
		{
			name:  "blank lines and trailing space are tolerated",
			reply: "\n[a/b.ts#1A2B]   \n\nPUT 12.=12:  \n-old\n+new\n\n",
			want: Hunk{
				Kind: KindPut,
				Line: 12,
				End:  12,
				Old:  []string{"old"},
				New:  []string{"new"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p, err := Parse(c.reply)

			g.Expect(err).NotTo(HaveOccurred())
			want := c.path
			if want == "" {
				want = "a/b.ts"
			}
			g.Expect(p.Path).To(Equal(want), "path from the header")
			g.Expect(p.Tag).To(Equal("1A2B"))
			g.Expect(p.Hunks).To(HaveLen(1))
			g.Expect(p.Hunks[0]).To(Equal(c.want))
		})
	}
}

func TestParseRefusesRatherThanGuessing(t *testing.T) {
	cases := []struct {
		name   string
		reply  string
		reason string
	}{
		{
			name:   "no header",
			reply:  "PUT 12.=12:\n-old\n+new",
			reason: "no section header",
		},
		{
			name:   "no hunk",
			reply:  "[a/b.ts#1A2B]",
			reason: "nothing to apply",
		},
		{
			name:   "minus row only",
			reply:  "[a/b.ts#1A2B]\nPUT 12.=12:\n-old",
			reason: "needs a `-` row and a `+` row",
		},
		{
			name:   "plus row only",
			reply:  "[a/b.ts#1A2B]\nPUT 12.=12:\n+new",
			reason: "needs a `-` row and a `+` row",
		},
		{
			name:   "bare body row",
			reply:  "[a/b.ts#1A2B]\nPUT 12.=12:\nold\n+new",
			reason: "must start with `-` or `+`",
		},
		{
			name:   "body row before any header",
			reply:  "[a/b.ts#1A2B]\n+new",
			reason: "before any `PUT` or `SUB`",
		},
		{
			name:   "minus row after a plus row",
			reply:  "[a/b.ts#1A2B]\nPUT 12.=12:\n-old\n+new\n-again",
			reason: "after a `+` row",
		},
		{
			name:   "line zero",
			reply:  "[a/b.ts#1A2B]\nPUT 0.=0:\n-old\n+new",
			reason: "run from 1",
		},
		{
			name:   "range ends before it starts",
			reply:  "[a/b.ts#1A2B]\nPUT 9.=4:\n-old\n+new",
			reason: "end before it starts",
		},
		{
			name:   "two different files",
			reply:  "[a/b.ts#1A2B]\nPUT 1.=1:\n-x\n+y\n[c/d.ts#1A2B]\nPUT 2.=2:\n-x\n+y",
			reason: "one file and one read",
		},
		{
			name:   "a PUT range with too few minus rows",
			reply:  "[a/b.ts#1A2B]\nPUT 10.=12:\n-only\n+one",
			reason: "needs 3 `-` rows and has 1",
		},
		{
			name:   "a PUT range with too many minus rows",
			reply:  "[a/b.ts#1A2B]\nPUT 10.=10:\n-one\n-two\n+x",
			reason: "needs 1 `-` rows and has 2",
		},
		{
			name:   "a blank line between body rows",
			reply:  "[a/b.ts#1A2B]\nPUT 10.=12:\n-first\n\n-third\n+one",
			reason: "a blank line between body rows",
		},
		{
			name:   "overlapping hunks",
			reply:  "[a/b.ts#1A2B]\nPUT 1.=3:\n-a\n-b\n-c\n+z\nPUT 2.=2:\n-b\n+w",
			reason: "not after the previous hunk",
		},
		{
			name:   "hunks out of order",
			reply:  "[a/b.ts#1A2B]\nPUT 5.=5:\n-e\n+f\nPUT 1.=1:\n-a\n+b",
			reason: "not after the previous hunk",
		},
		{
			name:   "a SUB with several rows either side",
			reply:  "[a/b.ts#1A2B]\nSUB 12:\n-a\n-b\n+c\n+d",
			reason: "one `-` row and one `+` row",
		},
		{
			name:   "prose instead of a patch",
			reply:  "I would change line 12 to say const n = 2.",
			reason: "this is not a patch",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p, err := Parse(c.reply)

			g.Expect(err).To(HaveOccurred(), "this reply must be refused")
			g.Expect(p).To(BeNil(), "a refused reply yields no patch to apply")
			g.Expect(err.Error()).To(ContainSubstring(c.reason))
			var f *Fault
			g.Expect(errors.As(err, &f)).
				To(BeTrue(), "a caller has to be able to tell a fault from any other error")
			g.Expect(f.Reason).To(ContainSubstring(c.reason))
		})
	}
}

// TestParseSkipsACodeFence records a tolerance rather than a rule.
//
// Models wrap a reply in a fence out of Markdown habit: 221 of 3,789 recorded
// replies arrive that way, and 81 of those were scored correct by the harness whose
// numbers chose this format. That harness matched its patterns anywhere in the text,
// so ignoring the fence is what every published success rate already assumes.
//
// It is safe because a fence cannot be anything else here. A body row is a sigil
// followed by text, and a header opens with a bracket, so a line of only backticks
// is neither.
func TestParseSkipsACodeFence(t *testing.T) {
	cases := []struct {
		name  string
		reply string
	}{
		{
			name:  "fenced with a language tag",
			reply: "```patch\n[a/b.ts#1A2B]\nPUT 12.=12:\n-old\n+new\n```",
		},
		{
			name:  "fenced with no tag",
			reply: "```\n[a/b.ts#1A2B]\nPUT 12.=12:\n-old\n+new\n```",
		},
		{
			name:  "only the closing fence survived",
			reply: "[a/b.ts#1A2B]\nPUT 12.=12:\n-old\n+new\n```",
		},
		{
			name:  "more than three backticks",
			reply: "````\n[a/b.ts#1A2B]\nPUT 12.=12:\n-old\n+new\n````",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			p, err := Parse(c.reply)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(p.Hunks).To(HaveLen(1))
			g.Expect(p.Hunks[0].Old).To(Equal([]string{"old"}))
			g.Expect(p.Hunks[0].New).To(Equal([]string{"new"}))
		})
	}
}

// TestAFenceBetweenBodyRowsIsRefused is the limit of that tolerance, and it mirrors
// the blank-line rule for the same reason: dropping a line inside a hunk silently
// shortens the replacement.
//
// A trailing fence is still skipped, because it is the closing wrapper. The two cases
// are told apart by whether a body row follows, and none of the 221 fenced replies
// recorded has one, so the tolerance covers every real reply and the refusal covers
// the shape that would corrupt a file.
func TestAFenceBetweenBodyRowsIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		reply   string
		refused bool
	}{
		{
			name:    "a fence between two replacement rows",
			reply:   "[a/b.md#D266]\nPUT 15.=15:\n-old\n+a\n```\n+b",
			refused: true,
		},
		{
			name:    "a fence between the old row and the new one",
			reply:   "[a/b.md#D266]\nPUT 15.=15:\n-old\n```\n+new",
			refused: true,
		},
		{
			name:    "a fence closing the reply, after the last row",
			reply:   "[a/b.md#D266]\nPUT 15.=15:\n-old\n+new\n```",
			refused: false,
		},
		{
			name:    "a fence opening the reply, before the header",
			reply:   "```patch\n[a/b.md#D266]\nPUT 15.=15:\n-old\n+new",
			refused: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := Parse(c.reply)

			if c.refused {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("a code fence between body rows"))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

// TestBackticksInContentSurvive is the other half. A fence is skipped only when the
// whole line is one; content that happens to contain backticks is behind a sigil and
// is not a fence.
func TestBackticksInContentSurvive(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "inline code in a comment",
			content: "// see `foo` for why",
		},
		{
			name:    "a fence as content, behind its sigil",
			content: "```",
		},
		{
			name:    "a fence with a tag, as content",
			content: "```go",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reply := "[a/b.ts#1A2B]\nPUT 12.=12:\n" + Row(SigilMinus, c.content) + "\n" + Row(SigilPlus, c.content+"!")

			p, err := Parse(reply)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(p.Hunks[0].Old).To(Equal([]string{c.content}))
			g.Expect(p.Hunks[0].New).To(Equal([]string{c.content + "!"}))
		})
	}
}
