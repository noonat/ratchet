package edit

import (
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"ratchet/internal/patch"
)

// diagnosis is the sentence a whitespace-only PUT mismatch adds.
const diagnosis = " The difference is whitespace only."

// TestTheNearMissDefinitionIsPinned fixes what counts as whitespace, on two shapes
// the fixtures do not contain. No record of the 323 differs in trailing spaces
// alone, and none holds non-ASCII whitespace, so neither case is evidence of damage
// a model does. Both are here to say what unicode.IsSpace covers, because the
// definition is the part a later reader would otherwise have to infer from the
// implementation.
func TestTheNearMissDefinitionIsPinned(t *testing.T) {
	cases := []struct {
		name string
		file string
		hunk patch.Hunk
	}{
		{
			name: "the row gained trailing spaces",
			file: "const n = 1;\n",
			hunk: patch.Hunk{Kind: patch.KindPut, Line: 1, End: 1,
				Old: []string{"const n = 1;   "}, New: []string{"const n = 2;"}},
		},
		{
			name: "the one differing space is a non-breaking space",
			file: "const n = 1;\n",
			hunk: patch.Hunk{Kind: patch.KindPut, Line: 1, End: 1,
				Old: []string{"const n =\u00a01;"}, New: []string{"const n = 2;"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(t.Context(), c.file, c.hunk)

			g.Expect(err).To(HaveOccurred())
			g.Expect(res.Text).To(BeEmpty(), "the diagnosis never turns a refusal into an edit")
			g.Expect(err.Error()).To(ContainSubstring(diagnosis))
		})
	}
}

// TestAPutNearMissSaysSo covers the damage the fixtures actually recorded. Of the
// 650 old_mismatch records, 323 differ from the file by whitespace alone: 318 in
// their leading whitespace and 5 in an internal space. Those are the shapes the
// sentence exists for, so they are the shapes pinned first.
func TestAPutNearMissSaysSo(t *testing.T) {
	cases := []struct {
		name string
		file string
		hunk patch.Hunk
	}{
		{
			name: "the row lost its indentation, which is 318 of the 323",
			file: "func f() {\n\treturn 1\n}\n",
			hunk: patch.Hunk{Kind: patch.KindPut, Line: 2, End: 2,
				Old: []string{"return 1"}, New: []string{"return 2"}},
		},
		{
			name: "the row differs in an internal space, which is the other 5",
			file: "const n = 1;\n",
			hunk: patch.Hunk{Kind: patch.KindPut, Line: 1, End: 1,
				Old: []string{"const n =  1;"}, New: []string{"const n =  2;"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(t.Context(), c.file, c.hunk)

			g.Expect(err).To(HaveOccurred())
			var r *Refusal
			g.Expect(errors.As(err, &r)).To(BeTrue())
			g.Expect(r.Reason).To(Equal(ReasonOldMismatch))
			g.Expect(err.Error()).To(ContainSubstring(diagnosis))
			g.Expect(res.Text).To(BeEmpty(), "a diagnosed near-miss is still a refusal")
		})
	}
}

// TestTheSentenceIsAboutTheRowItNames pins the per-row rule on a shape the corpus
// does not hold. Every recorded hunk carries one `-` row, but the parser accepts a
// multi-row range, so the rule has to be decided rather than left to the data.
//
// refuseMismatch returns at the first row that does not match, and the sentence is a
// claim about that row. A hunk whose second row lost an indent earns it even though
// the first row matched; a hunk whose first row is a content mismatch does not, even
// though a later row is whitespace apart, because the model is being sent to copy
// the row it was pointed at.
func TestTheSentenceIsAboutTheRowItNames(t *testing.T) {
	const file = "alpha\n    beta\ngamma\n"

	cases := []struct {
		name string
		old  []string
		want bool
	}{
		{
			name: "the named row is the near-miss, and the one before it matched",
			old:  []string{"alpha", "beta"},
			want: true,
		},
		{
			name: "the named row is a content mismatch and a later row is whitespace apart",
			old:  []string{"ALPHA", "beta"},
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(t.Context(), file, patch.Hunk{
				Kind: patch.KindPut, Line: 1, End: 2,
				Old: c.old, New: []string{"one", "two"},
			})

			g.Expect(err).To(HaveOccurred())
			g.Expect(res.Text).To(BeEmpty(), "the diagnosis never turns a refusal into an edit")
			if c.want {
				g.Expect(err.Error()).To(ContainSubstring(diagnosis))
				return
			}
			g.Expect(err.Error()).NotTo(ContainSubstring(diagnosis))
		})
	}
}

// subDiagnosis is the sentence a whitespace-only SUB near-miss adds.
const subDiagnosis = " Once whitespace is removed, it appears exactly once."

// TestASubNearMissSaysSo covers the fragment half. Of the 102 fragment_not_found
// records, 34 are a fragment absent byte-exact that occurs exactly once once
// whitespace is dropped, which is the only reachable near-miss: stripping never
// lowers an occurrence count, so a fragment present twice is present twice stripped
// and can never strip to one.
//
// The all-whitespace case is the guard rather than a shape a model produces. In Go
// strings.Count(s, "") is len(s)+1, so a fragment that strips to nothing would count
// as occurring exactly once on a line that also strips to nothing, and that is the
// one line where "appears once" is false.
func TestASubNearMissSaysSo(t *testing.T) {
	cases := []struct {
		name string
		file string
		frag string
		want bool
	}{
		{
			name: "absent byte-exact, and unique once whitespace is dropped",
			file: "const n = 1;\n",
			frag: "const  n",
			want: true,
		},
		{
			name: "present twice, so the strip cannot rescue it",
			file: "x = x + 1;\n",
			frag: "x",
			want: false,
		},
		{
			name: "a fragment that strips to nothing, on a line that also does",
			file: "\t\t\n",
			frag: " ",
			want: false,
		},
		{
			name: "absent for a reason that is not whitespace",
			file: "const n = 1;\n",
			frag: "let",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := applied(t.Context(), c.file, patch.Hunk{
				Kind: patch.KindSub, Line: 1, End: 1,
				Old: []string{c.frag}, New: []string{"replaced"},
			})

			g.Expect(err).To(HaveOccurred())
			g.Expect(res.Text).To(BeEmpty(), "a refused edit produces no file")
			if c.want {
				g.Expect(err.Error()).To(ContainSubstring(subDiagnosis))
				return
			}
			g.Expect(err.Error()).NotTo(ContainSubstring(subDiagnosis))
		})
	}
}

// TestAContentMismatchGetsNeitherSentence is the negative control for both halves.
// A refusal that says the difference is whitespace when it is not sends the model to
// retype a line that was wrong about its content, which is worse than saying nothing.
func TestAContentMismatchGetsNeitherSentence(t *testing.T) {
	g := NewWithT(t)
	res, err := applied(t.Context(), "const n = 1;\n", patch.Hunk{
		Kind: patch.KindPut, Line: 1, End: 1,
		Old: []string{"const m = 1;"}, New: []string{"const m = 2;"},
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(res.Text).To(BeEmpty(), "a content mismatch is a refusal, with or without a sentence")
	g.Expect(err.Error()).NotTo(ContainSubstring(diagnosis))
	g.Expect(err.Error()).NotTo(ContainSubstring(subDiagnosis))
}
