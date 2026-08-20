package edit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"ratchet/internal/anchor"
	"ratchet/internal/patch"
)

// numbered is a file of n lines, so a test can say which line it edited without
// spelling out the rest.
func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// diffOf applies the hunks and returns the diff, which is the only way to reach it:
// a diff is built from the hunks that produced it, not by comparing two files.
func diffOf(ctx context.Context, g *WithT, file string, hunks ...patch.Hunk) string {
	g.THelper()
	reads := anchor.NewReads()
	reads.Record("a/b.ts", anchor.NewSnapshot(file))

	res, err := Apply(ctx, reads, patch.Patch{
		Path:  "a/b.ts",
		Tag:   anchor.Tag(file),
		Hunks: hunks,
	}, file, Options{
		MaxHunks: len(hunks),
	})
	g.Expect(err).NotTo(HaveOccurred())
	return res.Diff
}

// replacing is one whole-line replacement.
func replacing(line int, old string, new ...string) patch.Hunk {
	return patch.Hunk{
		Kind: patch.KindPut,
		Line: line,
		End:  line,
		Old:  []string{old},
		New:  new,
	}
}

// TestDiffReportsOnlyWhatMoved is the property the field is documented as having, and
// the one a span between the first and last change does not: two edits eighteen lines
// apart reported thirty-four lines as changed, thirty of them identical.
func TestDiffReportsOnlyWhatMoved(t *testing.T) {
	cases := []struct {
		name  string
		lines int
		hunks []patch.Hunk
		want  string
	}{
		{
			name:  "one line of a short file",
			lines: 3,
			hunks: []patch.Hunk{replacing(2, "line 2", "TWO")},
			want:  "@@ -1,3 +1,3 @@\n line 1\n-line 2\n+TWO\n line 3\n",
		},
		{
			name:  "the first line",
			lines: 2,
			hunks: []patch.Hunk{replacing(1, "line 1", "ONE")},
			want:  "@@ -1,2 +1,2 @@\n-line 1\n+ONE\n line 2\n",
		},
		{
			name:  "one line becomes three",
			lines: 3,
			hunks: []patch.Hunk{replacing(2, "line 2", "a", "b", "c")},
			want:  "@@ -1,3 +1,5 @@\n line 1\n-line 2\n+a\n+b\n+c\n line 3\n",
		},
		{
			name:  "two edits far apart get a block each",
			lines: 20,
			hunks: []patch.Hunk{replacing(2, "line 2", "TWO"), replacing(18, "line 18", "EIGHTEEN")},
			want: "@@ -1,5 +1,5 @@\n line 1\n-line 2\n+TWO\n line 3\n line 4\n line 5\n" +
				"@@ -15,6 +15,6 @@\n line 15\n line 16\n line 17\n-line 18\n+EIGHTEEN\n line 19\n line 20\n",
		},
		{
			name:  "two edits close together share a block, and the lines between stay context",
			lines: 20,
			hunks: []patch.Hunk{replacing(5, "line 5", "FIVE"), replacing(8, "line 8", "EIGHT")},
			want: "@@ -2,10 +2,10 @@\n line 2\n line 3\n line 4\n-line 5\n+FIVE\n" +
				" line 6\n line 7\n-line 8\n+EIGHT\n line 9\n line 10\n line 11\n",
		},
		{
			name:  "adjacent edits group their removals before their additions",
			lines: 20,
			hunks: []patch.Hunk{replacing(5, "line 5", "FIVE"), replacing(6, "line 6", "SIX")},
			want: "@@ -2,8 +2,8 @@\n line 2\n line 3\n line 4\n-line 5\n-line 6\n+FIVE\n+SIX\n" +
				" line 7\n line 8\n line 9\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(diffOf(t.Context(), g, numbered(c.lines), c.hunks...)).To(Equal(c.want))
		})
	}
}

// TestDiffCountsMatchWhatItPrints checks the `@@` header against the body, because a
// header nobody verifies is a header that drifts.
func TestDiffCountsMatchWhatItPrints(t *testing.T) {
	cases := []struct {
		name  string
		hunks []patch.Hunk
	}{
		{name: "one line replaced", hunks: []patch.Hunk{replacing(10, "line 10", "TEN")}},
		{name: "one line becomes four", hunks: []patch.Hunk{replacing(10, "line 10", "a", "b", "c", "d")}},
		{name: "two blocks", hunks: []patch.Hunk{replacing(3, "line 3", "THREE"), replacing(17, "line 17", "SEVENTEEN")}},
		{name: "one merged block", hunks: []patch.Hunk{replacing(9, "line 9", "x", "y"), replacing(12, "line 12", "TWELVE")}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			for _, blk := range strings.Split(diffOf(t.Context(), g, numbered(20), c.hunks...), "@@ ")[1:] {
				head, body, ok := strings.Cut(blk, " @@\n")
				g.Expect(ok).To(BeTrue())

				var oldFrom, oldCount, newFrom, newCount int
				_, err := fmt.Sscanf(head, "-%d,%d +%d,%d", &oldFrom, &oldCount, &newFrom, &newCount)
				g.Expect(err).NotTo(HaveOccurred())

				olds, news := 0, 0
				for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
					switch line[0] {
					case '-':
						olds++
					case '+':
						news++
					default:
						olds++
						news++
					}
				}
				g.Expect(olds).To(Equal(oldCount), "header claims %d old lines, body has %d", oldCount, olds)
				g.Expect(news).To(Equal(newCount), "header claims %d new lines, body has %d", newCount, news)
			}
		})
	}
}

// TestApplyCarriesADiffOnlyWhenItApplied keeps the field honest: a refusal changed
// nothing, so a diff of it would describe an edit that did not happen.
func TestApplyCarriesADiffOnlyWhenItApplied(t *testing.T) {
	g := NewWithT(t)
	file := "one\ntwo\nthree\n"
	reads := anchor.NewReads()
	reads.Record("a/b.ts", anchor.NewSnapshot(file))

	res, err := Apply(t.Context(), reads, patch.Patch{
		Path:  "a/b.ts",
		Tag:   anchor.Tag(file),
		Hunks: []patch.Hunk{replacing(2, "NOT two", "TWO")},
	}, file, Options{})

	g.Expect(err).To(HaveOccurred())
	var r *Refusal
	g.Expect(errors.As(err, &r)).To(BeTrue())
	g.Expect(r.Reason).To(Equal(ReasonOldMismatch))
	g.Expect(res.Diff).To(BeEmpty())
}
