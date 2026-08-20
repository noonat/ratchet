package replay

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/dev/fixture"
)

// TestBuildPutsTheLineWhereItWasRecorded is the assumption every number in the report
// rests on. If the addressed line is not at its recorded number, the applier compares
// against filler and a correct reply reads as wrong.
func TestBuildPutsTheLineWhereItWasRecorded(t *testing.T) {
	cases := []struct {
		name     string
		line     int
		original string
	}{
		{
			name:     "the first line",
			line:     1,
			original: "'use strict';",
		},
		{
			name:     "a line well into the file",
			line:     141,
			original: "      if (Math.random() < 0.5) {",
		},
		{
			name:     "content that looks like a body row",
			line:     15,
			original: "- **a markdown bullet**",
		},
		{
			name:     "content that is only a code fence",
			line:     7,
			original: "```",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			file := Build(c.line, c.original)

			got, ok := Line(file, c.line)
			g.Expect(ok).To(BeTrue())
			g.Expect(got).To(Equal(c.original))
			g.Expect(strings.Count(file, "\n")).
				To(Equal(c.line+Tail), "the file is the line, what precedes it, and the tail")
		})
	}
}

func TestLineCountsWhatTheFileShows(t *testing.T) {
	cases := []struct {
		name string
		text string
		at   int
		want string
		ok   bool
	}{
		{
			name: "the only line",
			text: "a\n",
			at:   1,
			want: "a",
			ok:   true,
		},
		{
			name: "a trailing newline does not start another line",
			text: "a\n",
			at:   2,
			ok:   false,
		},
		{
			name: "an unterminated last line still counts",
			text: "a\nb",
			at:   2,
			want: "b",
			ok:   true,
		},
		{
			name: "a blank line in the middle",
			text: "a\n\nc\n",
			at:   2,
			want: "",
			ok:   true,
		},
		{
			name: "past the end",
			text: "a\nb\n",
			at:   9,
			ok:   false,
		},
		{
			name: "an empty file has no lines",
			text: "",
			at:   1,
			ok:   false,
		},
		{
			name: "line zero is not a line",
			text: "a\n",
			at:   0,
			ok:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, ok := Line(c.text, c.at)

			g.Expect(ok).To(Equal(c.ok))
			if c.ok {
				g.Expect(got).To(Equal(c.want))
			}
		})
	}
}

// TestReplayClassifies pins the mapping from what the applier did to the vocabulary
// the harness used, because every disagreement is read through it.
func TestReplayClassifies(t *testing.T) {
	cases := []struct {
		name string
		rec  fixture.Record
		want Verdict
	}{
		{
			name: "a clean substitution",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "let n = 1;",
				Reply:    "[a/b.ts#1A2B]\nSUB 12:\n-const\n+let",
			},
			want: VerdictCorrect,
		},
		{
			name: "a clean whole-line replacement",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 1;\n+const n = 2;",
			},
			want: VerdictCorrect,
		},
		{
			name: "the old row does not match the line",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 9;\n+const n = 2;",
			},
			want: VerdictRefused,
		},
		{
			name: "the edit applies and produces the wrong text",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 1;\n+const n = 3;",
			},
			want: VerdictAppliedWrong,
		},
		{
			name: "nothing parseable",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "I am sorry, I cannot find that line.",
			},
			want: VerdictMalformed,
		},
		{
			name: "more hunks than were asked for",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 1;\n+const n = 2;\nPUT 14.=14:\n-x\n+y",
			},
			want: VerdictAppliedWrong,
		},
		{
			name: "a line other than the one recorded",
			rec: fixture.Record{
				Line:     12,
				Original: "const n = 1;",
				Want:     "const n = 2;",
				Reply:    "[a/b.ts#1A2B]\nPUT 40.=40:\n-const n = 1;\n+const n = 2;",
			},
			want: VerdictUnreplayable,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got := Replay(c.rec)

			g.Expect(got.Verdict).To(Equal(c.want), "detail was: %s", got.Detail)
		})
	}
}

// TestAnAbstentionIsNotADisagreement is the property that keeps the reported
// agreement honest: a reply nothing here can judge is counted out of the total
// rather than against the applier.
func TestAnAbstentionIsNotADisagreement(t *testing.T) {
	g := NewWithT(t)
	records := []fixture.Record{
		{
			Form:     "put_diff_checked",
			Line:     12,
			Original: "const n = 1;",
			Want:     "const n = 2;",
			Reply:    "[a/b.ts#1A2B]\nPUT 12.=12:\n-const n = 1;\n+const n = 2;",
			Outcome:  "correct",
		},
		{
			Form:     "put_diff_checked",
			Line:     12,
			Original: "const n = 1;",
			Want:     "const n = 2;",
			Reply:    "[a/b.ts#1A2B]\nPUT 40.=40:\n-const n = 1;\n+const n = 2;",
			Outcome:  "applied_wrong",
		},
	}

	rep := Run(records)

	g.Expect(rep.Tallies).To(HaveLen(1))
	t0 := rep.Tallies[0]
	g.Expect(t0.All.Records).To(Equal(2))
	g.Expect(t0.All.Abstained).To(Equal(1))
	g.Expect(t0.All.Judged()).To(Equal(1))
	g.Expect(t0.All.Agreed).To(Equal(1))
	g.Expect(rep.Disagreements).To(BeEmpty(), "an abstention is not a disagreement")

	share, measured := t0.All.Share()
	g.Expect(measured).To(BeTrue())
	g.Expect(share).To(Equal(100.0))
}

// TestShareSaysWhenNothingWasJudged separates no agreement from no data.
func TestShareSaysWhenNothingWasJudged(t *testing.T) {
	g := NewWithT(t)

	_, measured := Counts{Records: 3, Abstained: 3}.Share()

	g.Expect(measured).To(BeFalse())
}

// TestDuplicatesCountTwiceInAllAndOnceInDistinct is the decision the report exists to
// keep visible: the same reply arriving from several models is weighted by frequency
// in one column and by coverage in the other.
func TestDuplicatesCountTwiceInAllAndOnceInDistinct(t *testing.T) {
	g := NewWithT(t)
	one := fixture.Record{
		Form:     "sub_diff",
		Fixture:  "game",
		Line:     12,
		Original: "const n = 1;",
		Want:     "let n = 1;",
		Reply:    "[a/b.ts#1A2B]\nSUB 12:\n-const\n+let",
		Outcome:  "correct",
	}

	rep := Run([]fixture.Record{one, one, one})

	g.Expect(rep.Tallies).To(HaveLen(1))
	g.Expect(rep.Tallies[0].All.Records).To(Equal(3))
	g.Expect(rep.Tallies[0].Distinct.Records).To(Equal(1))
}
