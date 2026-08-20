package edit

import (
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"

	"ratchet/internal/anchor"
	"ratchet/internal/patch"
)

const served = "one\ntwo\nthree\n"

// put builds a single-line PUT against a path and tag, for the tables below.
func put(path, tag string, line int, old, new string) patch.Patch {
	return patch.Patch{
		Path: path,
		Tag:  tag,
		Hunks: []patch.Hunk{
			{
				Kind: patch.KindPut,
				Line: line,
				End:  line,
				Old:  []string{old},
				New:  []string{new},
			},
		},
	}
}

func TestResolveRefusalBranches(t *testing.T) {
	cases := []struct {
		name     string
		record   bool
		partial  []int
		current  string
		tag      string
		line     int
		hunkless bool
		want     Reason
		ok       bool
	}{
		{
			name:    "the file is what was served and the anchor matches",
			record:  true,
			current: served,
			tag:     anchor.Tag(served),
			line:    2,
			ok:      true,
		},
		{
			name:     "a patch with no hunks asks for nothing",
			record:   true,
			current:  served,
			tag:      anchor.Tag(served),
			hunkless: true,
			want:     ReasonUnusable,
		},
		{
			name:    "no read issued an anchor for this path",
			record:  false,
			current: served,
			tag:     anchor.Tag(served),
			line:    2,
			want:    ReasonNoRead,
		},
		{
			name:    "the anchor is wrong and the file has not moved",
			record:  true,
			current: served,
			tag:     "0000",
			line:    2,
			want:    ReasonMistranscribed,
		},
		{
			name:    "the anchor is right and the file has moved",
			record:  true,
			current: "one\nCHANGED\nthree\n",
			tag:     anchor.Tag(served),
			line:    2,
			want:    ReasonFileMoved,
		},
		{
			name:    "the anchor is wrong and the file has moved",
			record:  true,
			current: "one\nCHANGED\nthree\n",
			tag:     "0000",
			line:    2,
			want:    ReasonFileMoved,
		},
		{
			name:    "the read did not show the line being edited",
			record:  true,
			partial: []int{1, 3},
			current: served,
			tag:     anchor.Tag(served),
			line:    2,
			want:    ReasonLineNotShown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reads := anchor.NewReads()
			if c.record {
				snap := anchor.NewSnapshot(served)
				if c.partial != nil {
					snap = anchor.NewSnapshotForLines(served, c.partial)
				}
				reads.Record("a/b.ts", snap)
			}

			p := put("a/b.ts", c.tag, c.line, "two", "TWO")
			if c.hunkless {
				p.Hunks = nil
			}

			_, err := Resolve(reads, p, c.current)

			if c.ok {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			g.Expect(err).To(HaveOccurred())
			var r *Refusal
			g.Expect(errors.As(err, &r)).To(BeTrue(), "a refusal has to be recognizable to a caller")
			g.Expect(r.Reason).To(Equal(c.want), "message was: %s", r.Message)
		})
	}
}

// TestMovedFileRefusalNamesNoAnchor is the whole scheme in one assertion. The stale
// branch shows content and hands back nothing copyable: an error that answered with
// the file's current anchor would tell the model to edit content it has never seen,
// which is the silent wrong-line edit arriving through the rejection instead of
// through the edit.
func TestMovedFileRefusalNamesNoAnchor(t *testing.T) {
	moved := "one\nCHANGED\nthree\n"

	cases := []struct {
		name string
		tag  string
	}{
		{
			name: "anchor correct, file moved",
			tag:  anchor.Tag(served),
		},
		{
			name: "anchor wrong, file moved",
			tag:  "0000",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reads := anchor.NewReads()
			reads.Record("a/b.ts", anchor.NewSnapshot(served))

			_, err := Resolve(reads, put("a/b.ts", c.tag, 2, "two", "TWO"), moved)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).NotTo(ContainSubstring(anchor.Tag(moved)), "the refusal handed back the anchor of a file the model has not read")
			g.Expect(err.Error()).NotTo(ContainSubstring(anchor.Tag(served)), "the refusal repeated an anchor that no longer addresses anything")
			g.Expect(err.Error()).To(ContainSubstring("CHANGED"), "the refusal has to show what is actually there")
		})
	}
}

// TestMistranscribedRefusalNamesTheAnchor is the one branch permitted to. Nothing
// moved, so the mismatch can only be a transcription error and the resolver knows
// what was meant.
func TestMistranscribedRefusalNamesTheAnchor(t *testing.T) {
	g := NewWithT(t)
	reads := anchor.NewReads()
	reads.Record("a/b.ts", anchor.NewSnapshot(served))

	_, err := Resolve(reads, put("a/b.ts", "0000", 2, "two", "TWO"), served)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(anchor.Tag(served)))
}

// TestTrimmedWhitespaceIsNotAMovedFile is the reason the moved check compares tags
// rather than bytes. The tag ignores trailing whitespace and line endings on purpose,
// so an editor that trims on save does not invalidate an anchor it never saw.
// Comparing bytes refused the edit and printed a window byte-identical to what was
// displayed, telling the model to re-read a file that would say the same thing.
func TestTrimmedWhitespaceIsNotAMovedFile(t *testing.T) {
	cases := []struct {
		name    string
		current string
	}{
		{
			name:    "trailing whitespace trimmed on save",
			current: "one\ntwo\nthree\n",
		},
		{
			name:    "re-checked out with CRLF",
			current: "one   \r\ntwo\r\nthree\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			trailing := "one   \ntwo\nthree\n"
			g.Expect(anchor.Tag(c.current)).
				To(Equal(anchor.Tag(trailing)), "the fixture has to be one the tag calls unchanged")

			reads := anchor.NewReads()
			reads.Record("a/b.ts", anchor.NewSnapshot(trailing))

			_, err := Resolve(reads, put("a/b.ts", anchor.Tag(trailing), 2, "two", "TWO"), c.current)

			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

// TestResolveRefusesAnUnusablePatch covers the shapes Parse rejects and a patch built
// in code does not. Each one indexed a row or a range directly, so unchecked each was
// a panic rather than a refusal.
func TestResolveRefusesAnUnusablePatch(t *testing.T) {
	cases := []struct {
		name  string
		hunks []patch.Hunk
	}{
		{
			name: "hunks out of order",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 3, End: 3, Old: []string{"three"}, New: []string{"THREE"}},
				{Kind: patch.KindPut, Line: 1, End: 1, Old: []string{"one"}, New: []string{"ONE"}},
			},
		},
		{
			name: "hunks overlapping",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 2, Old: []string{"one", "two"}, New: []string{"x"}},
				{Kind: patch.KindPut, Line: 2, End: 2, Old: []string{"two"}, New: []string{"y"}},
			},
		},
		{
			name: "a range that ends before it starts",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 3, End: 1, Old: []string{"three"}, New: []string{"THREE"}},
			},
		},
		{
			name: "a SUB with no rows",
			hunks: []patch.Hunk{
				{Kind: patch.KindSub, Line: 1, End: 1},
			},
		},
		{
			name: "a SUB with two fragments",
			hunks: []patch.Hunk{
				{Kind: patch.KindSub, Line: 1, End: 1, Old: []string{"a", "b"}, New: []string{"c", "d"}},
			},
		},
		{
			name: "a PUT stating fewer lines than its range covers",
			hunks: []patch.Hunk{
				{Kind: patch.KindPut, Line: 1, End: 2, Old: []string{"one"}, New: []string{"x"}},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			reads := anchor.NewReads()
			reads.Record("a/b.ts", anchor.NewSnapshot(served))

			res, err := Apply(t.Context(), reads, patch.Patch{
				Path:  "a/b.ts",
				Tag:   anchor.Tag(served),
				Hunks: c.hunks,
			}, served, Options{MaxHunks: len(c.hunks)})

			g.Expect(err).To(HaveOccurred())
			var r *Refusal
			g.Expect(errors.As(err, &r)).To(BeTrue())
			g.Expect(r.Reason).To(Equal(ReasonUnusable), "message was: %s", r.Message)
			g.Expect(res.Text).To(BeEmpty())
		})
	}
}
