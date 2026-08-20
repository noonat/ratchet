package edit

import (
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"ratchet/internal/anchor"
	"ratchet/internal/patch"
)

// applied runs a patch against a file that was read whole, which is the case every
// test here starts from.
func applied(file string, hunks ...patch.Hunk) (Result, error) {
	reads := anchor.NewReads()
	reads.Record("a/b.ts", anchor.NewSnapshot(file))
	return Apply(reads, patch.Patch{
		Path:  "a/b.ts",
		Tag:   anchor.Tag(file),
		Hunks: hunks,
	}, file)
}

func TestApplyProducesTheEditedFile(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		hunks []patch.Hunk
		want  string
	}{
		{
			name: "a whole line replaced",
			file: "one\ntwo\nthree\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"two"}, New: []string{"TWO"}},
			},
			want: "one\nTWO\nthree\n",
		},
		{
			name: "a fragment replaced within a line",
			file: "const n = 1;\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindSub, Line: 1, End: 1, Old: []string{"const"}, New: []string{"let"}},
			},
			want: "let n = 1;\n",
		},
		{
			name: "a range replaced by fewer lines",
			file: "a\nb\nc\nd\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 2, End: 3, Old: []string{"b", "c"}, New: []string{"bc"}},
			},
			want: "a\nbc\nd\n",
		},
		{
			name: "two hunks, both addressing original line numbers",
			file: "a\nb\nc\nd\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 1, Old: []string{"a"}, New: []string{"a1", "a2"}},
				{Kind: patch.KindPut, Line: 4, End: 4, Old: []string{"d"}, New: []string{"D"}},
			},
			want: "a1\na2\nb\nc\nD\n",
		},
		{
			name: "no trailing newline is not invented",
			file: "one\ntwo",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"two"}, New: []string{"TWO"}},
			},
			want: "one\nTWO",
		},
		{
			name: "a CRLF file keeps CRLF on every line",
			file: "one\r\ntwo\r\nthree\r\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"two"}, New: []string{"TWO"}},
			},
			want: "one\r\nTWO\r\nthree\r\n",
		},
		{
			name: "a mixed-ending file keeps every line's own ending",
			file: "one\ntwo\r\nthree\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 1, Old: []string{"one"}, New: []string{"ONE"}},
			},
			want: "ONE\ntwo\r\nthree\n",
		},
		{
			name: "replacing an unterminated last line does not invent a newline",
			file: "one\ntwo",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"two"}, New: []string{"a", "b"}},
			},
			want: "one\na\nb",
		},
		{
			name: "an insertion into a CRLF file is CRLF",
			file: "one\r\ntwo\r\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 1, Old: []string{"one"}, New: []string{"a", "b"}},
			},
			want: "a\r\nb\r\ntwo\r\n",
		},
		{
			name: "a replacement is written exactly as it was sent",
			file: "a\n",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 1, Old: []string{"a"}, New: []string{"x — y"}},
			},
			want: "x — y\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(c.file, c.hunks...)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.Text).To(Equal(c.want))
			g.Expect(res.Now).To(Equal(c.file), "the file as it stands is always handed back")
		})
	}
}

// TestNothingIsRewrittenBehindTheModel pins the absence of a normalization stage.
//
// A Unicode table was specified here and would have converted an em dash to a
// hyphen. Measured against 19,055 recorded replies, no model ever introduced one;
// every reply containing one had copied it out of a file that already had it, and
// converting would have turned 289 correct edits into wrong ones. Both the line the
// edit names and the lines it does not are written through unchanged.
func TestNothingIsRewrittenBehindTheModel(t *testing.T) {
	g := NewWithT(t)
	file := "keep — this\nchange me\n"

	res, err := applied(file, patch.Hunk{
		Kind: patch.KindPut,
		Line: 2,
		End:  2,
		Old:  []string{"change me"},
		New:  []string{"changed"},
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res.Text).To(Equal("keep — this\nchanged\n"))
}

func TestApplyRefusesAndSaysWhat(t *testing.T) {
	cases := []struct {
		name      string
		file      string
		hunk      patch.Hunk
		want      Reason
		wantWould string
	}{
		{
			name:      "the line is not what the hunk says it is",
			file:      "one\ntwo\nthree\n",
			hunk:      patch.Hunk{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"TWO"}, New: []string{"2"}},
			want:      ReasonOldMismatch,
			wantWould: "one\n2\nthree\n",
		},
		{
			name:      "the fragment appears twice, so which one is unknowable",
			file:      "x = x + 1;\n",
			hunk:      patch.Hunk{Kind: patch.KindSub, Line: 1, End: 1, Old: []string{"x"}, New: []string{"y"}},
			want:      ReasonOldMismatch,
			wantWould: "y = x + 1;\n",
		},
		{
			name:      "the fragment is not on the line at all",
			file:      "const n = 1;\n",
			hunk:      patch.Hunk{Kind: patch.KindSub, Line: 1, End: 1, Old: []string{"let"}, New: []string{"var"}},
			want:      ReasonOldMismatch,
			wantWould: "const n = 1;\n",
		},
		{
			name: "the file has no such line",
			file: "one\ntwo\n",
			hunk: patch.Hunk{Kind: patch.KindPut, Line: 9, End: 9, Old: []string{"nine"}, New: []string{"9"}},
			want: ReasonOutOfRange,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(c.file, c.hunk)

			g.Expect(err).To(HaveOccurred())
			var r *Refusal
			g.Expect(errors.As(err, &r)).To(BeTrue())
			g.Expect(r.Reason).To(Equal(c.want))
			g.Expect(res.Text).To(BeEmpty(), "a refused edit produces no file")
			g.Expect(res.Now).To(Equal(c.file), "the file as it stands is always handed back")
			g.Expect(res.Would).To(Equal(c.wantWould), "the model has to see its own attempt, or it re-sends it")
		})
	}
}

// TestAnchorRefusalShowsNoAttempt separates the two kinds of refusal. A content
// refusal hands back the attempt, because the file it was spliced into is the file
// the model read. An anchor refusal cannot: splicing into a file the model has not
// read is the wrong-line edit the anchor exists to prevent.
func TestAnchorRefusalShowsNoAttempt(t *testing.T) {
	g := NewWithT(t)
	reads := anchor.NewReads()
	reads.Record("a/b.ts", anchor.NewSnapshot("one\ntwo\nthree\n"))
	moved := "one\nCHANGED\nthree\n"

	res, err := Apply(reads, put("a/b.ts", anchor.Tag("one\ntwo\nthree\n"), 2, "two", "TWO"), moved)

	g.Expect(err).To(HaveOccurred())
	g.Expect(res.Would).To(BeEmpty())
	g.Expect(res.Now).To(Equal(moved))
}
