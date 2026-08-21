package fixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/gomega"
)

// build runs a rebuild into a temporary tree and saves the result, so a test can
// then run a second rebuild against what the first one wrote.
func build(g *WithT, dir, path string, force bool) (*Set, error) {
	g.THelper()
	set, err := Rebuild(dir, path, force)
	if err != nil {
		return nil, err
	}
	g.Expect(Save(path, set)).To(Succeed())
	return set, nil
}

// TestAnEmptyJournalDirectoryChangesNothing is the case the gitignore creates and
// the one that would otherwise destroy the fixtures. A fresh clone has the file and
// none of its sources, so a rebuild there has to be a no-op rather than a wipe.
func TestAnEmptyJournalDirectoryChangesNothing(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "a.jsonl", []string{
		row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(g, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
	})
	built, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(built.Records).To(HaveLen(2))
	before, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	// The fresh clone: the fixtures are committed, the journals are not.
	g.Expect(os.Remove(filepath.Join(journals, "a.jsonl"))).To(Succeed())
	rebuilt, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rebuilt.Records).To(HaveLen(2), "records whose journal is absent are kept")

	after, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(after)).To(Equal(string(before)), "a rebuild with no journals must not change a byte")
}

// TestRebuildingTwiceProducesTheSameBytes is what makes the file reviewable: a
// rebuild that reorders records would show as a whole-file diff every time.
func TestRebuildingTwiceProducesTheSameBytes(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "b.jsonl", []string{row(g, "sub_diff", "game", 5, "e", "f", "r3", "correct")})
	journal(g, journals, "a.jsonl", []string{row(g, "put_diff_checked", "textwrap", 1, "a", "b", "r1", "refused")})

	_, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	pass1, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	pass2, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(string(pass2)).To(Equal(string(pass1)))
}

// TestRebuildRefusesARescoredJournal covers the surprise a person causes: the
// journal was rerun or rescored, so the records built from it would assert
// something other than what the committed fixtures assert.
func TestRebuildRefusesARescoredJournal(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "a.jsonl", []string{
		row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(g, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	before, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	journal(g, journals, "a.jsonl", []string{
		row(g, "sub_diff", "game", 1, "a", "b", "r1", "applied_wrong"),
		row(g, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
	})

	_, err = Rebuild(journals, path, false)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("has changed since the fixtures were built"))

	after, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(after)).To(Equal(string(before)), "a refused rebuild leaves the file alone")

	_, err = Rebuild(journals, path, true)
	g.Expect(err).NotTo(HaveOccurred(), "FORCE=1 is the caller saying the change is intended")
}

// TestRebuildRefusesToKeepLessThanItDid covers the surprise the code causes, and it
// is a different guard from the hash.
//
// A journal that loses rows also changes hash, so the hash check catches that first.
// What reaches this one is a distiller that keeps less than it used to: a form left
// Forms, or Extract grew stricter. The journals are then all unchanged and the file
// quietly shrinks, which is why the count is recorded per journal in the header.
func TestRebuildRefusesToKeepLessThanItDid(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "a.jsonl", []string{
		row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(g, "put_diff_checked", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	before, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	restore := Forms
	Forms = map[string]struct{}{"sub_diff": {}}
	defer func() { Forms = restore }()

	_, err = Rebuild(journals, path, false)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("keeps less of it than it did"))

	after, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(after)).To(Equal(string(before)))

	_, err = Rebuild(journals, path, true)
	g.Expect(err).NotTo(HaveOccurred())
}

// TestANewJournalIsAdded is the ordinary case: a run finishes, its journal is
// copied in, and its records join the ones already there.
func TestANewJournalIsAdded(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "a.jsonl", []string{row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct")})
	_, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())

	journal(g, journals, "b.jsonl", []string{row(g, "put_diff_checked", "bullets", 7, "g", "h", "r2", "refused")})
	set, err := build(g, journals, path, false)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(set.Records).To(HaveLen(2))
	g.Expect(set.Sources).To(HaveLen(2))
}

// TestRebuildRefusesASupersededJournal enforces what journals/README.md asks for.
// The guards cannot see this one: a superseded journal is a new source with an
// unchanged hash and a growing count, so every reply it shares with its replacement
// is counted twice.
func TestRebuildRefusesASupersededJournal(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	// The row varies by name, so a second entry in Superseded is not an identical
	// pair. Identical content under two names is its own refusal, and this test
	// forces a rebuild past the superseded one.
	for name := range Superseded {
		journal(g, journals, name, []string{
			row(g, "sub_diff", "game", 1, name, "b", "r1", "correct"),
		})
	}

	_, err := Rebuild(journals, path, false)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("was superseded by"))

	_, err = Rebuild(journals, path, true)
	g.Expect(err).NotTo(HaveOccurred(), "FORCE=1 is the caller saying they meant it")
}

// TestRebuildRefusesAJournalThatKeepsLessOnItsOwn is the guard the total cannot
// give. A journal added in the same run can more than cover another one shrinking,
// so the total grows while records vanish and the header rewrites the count that
// would have shown it.
func TestRebuildRefusesAJournalThatKeepsLessOnItsOwn(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	journal(g, journals, "a.jsonl", []string{
		row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(g, "put_diff_checked", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())

	// a.jsonl now yields one record instead of two, and b.jsonl more than covers it
	restore := Forms
	Forms = map[string]struct{}{"sub_diff": {}}
	defer func() { Forms = restore }()
	journal(g, journals, "b.jsonl", []string{
		row(g, "sub_diff", "bullets", 7, "g", "h", "r3", "refused"),
		row(g, "sub_diff", "bullets", 8, "i", "j", "r4", "refused"),
		row(g, "sub_diff", "bullets", 9, "k", "l", "r5", "refused"),
	})

	set, err := Rebuild(journals, path, false)
	g.Expect(err).To(HaveOccurred(), "the total grew, so only a per-journal count can catch this")
	g.Expect(err.Error()).To(ContainSubstring("keeps less of it than it did"))
	g.Expect(set).To(BeNil())

	set, err = Rebuild(journals, path, true)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(set.Records).To(HaveLen(4), "one from a.jsonl and three from b.jsonl")
}

// TestARefusalIsTypedSoItPrintsAsAMessage keeps the two error kinds apart. A refusal
// is written for whoever ran the target; a fault carries a stack that says which of
// several file operations produced it.
func TestARefusalIsTypedSoItPrintsAsAMessage(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())

	for name := range Superseded {
		journal(g, journals, name, []string{row(g, "sub_diff", "game", 1, name, "b", "r", "correct")})
	}

	_, err := Rebuild(journals, filepath.Join(dir, "fixtures.jsonl"), false)

	var refusal *Refusal
	g.Expect(errors.As(err, &refusal)).To(BeTrue())
	g.Expect(refusal.Reason).NotTo(BeEmpty())
}

// TestRebuildRefusesASecondNameOnTheSameContent is the corruption this guard exists
// for. A journal renamed outside the repo arrives with a hash no recorded source
// claims, its old name is absent so its records are carried over, and every reply it
// holds is then counted twice. Nothing at rebuild time says so: the hashes are
// unchanged and the totals only grow, so neither the rescored check nor the shrink
// check nor the backstop can see it.
func TestRebuildRefusesASecondNameOnTheSameContent(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	journals := filepath.Join(dir, "journals")
	g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
	path := filepath.Join(dir, "fixtures.jsonl")

	rows := []string{row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct")}
	journal(g, journals, "edit-old.jsonl", rows)
	first, err := build(g, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())

	// The rename: same bytes, new name, the old name gone from the directory.
	g.Expect(os.Remove(filepath.Join(journals, "edit-old.jsonl"))).To(Succeed())
	journal(g, journals, "edit-new.jsonl", rows)

	_, err = Rebuild(journals, path, false)

	g.Expect(err).To(HaveOccurred(), "a rebuild that doubles every record must refuse")
	g.Expect(err.Error()).To(ContainSubstring("edit-new.jsonl"))
	g.Expect(err.Error()).To(ContainSubstring("edit-old.jsonl"), "the refusal names both")
	g.Expect(err.Error()).To(ContainSubstring("twice"))
	g.Expect(err.Error()).To(ContainSubstring("edit-old.jsonl"), "and offers the recorded name")
	g.Expect(len(first.Records)).To(Equal(1), "the fixture holds one record before the rename")
}

// TestRebuildRefusesEveryShapeOfADuplicate walks the states a second name can be in,
// because each one has a different remedy and the wrong remedy sends somebody to the
// wrong file. The fact that selects it is whether this name is already recorded under
// this very hash: if it is, the doubling is in the committed header; if it is not, the
// content is a copy sitting on disk.
//
// force is passed as true throughout. The other rebuild refusals accept it, because
// the evidence under a named source can genuinely change. This one does not, so the
// cases that refuse must refuse anyway.
func TestRebuildRefusesEveryShapeOfADuplicate(t *testing.T) {
	same := func(g *WithT) []string {
		g.THelper()
		return []string{row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct")}
	}
	other := func(g *WithT) []string {
		g.THelper()
		return []string{row(g, "put_diff_checked", "textwrap", 4, "c", "d", "r2", "refused")}
	}

	cases := []struct {
		name   string
		setUp  func(g *WithT, journals, path string)
		refuse bool
		remedy string
	}{
		{
			name: "two new journals, identical content, neither recorded",
			setUp: func(g *WithT, journals, path string) {
				journal(g, journals, "edit-aaa.jsonl", same(g))
				journal(g, journals, "edit-zzz.jsonl", same(g))
			},
			refuse: true,
			remedy: "Remove one of them",
		},
		{
			name: "the same content under two names, one of them recorded under it",
			setUp: func(g *WithT, journals, path string) {
				journal(g, journals, "edit-mmm.jsonl", same(g))
				_, err := build(g, journals, path, false)
				g.Expect(err).NotTo(HaveOccurred())
				journal(g, journals, "edit-zzz.jsonl", same(g))
			},
			refuse: true,
			remedy: "Remove one of them",
		},
		{
			name: "the fixtures already hold two names under one hash",
			setUp: func(g *WithT, journals, path string) {
				journal(g, journals, "edit-foo.jsonl", same(g))
				set, err := build(g, journals, path, false)
				g.Expect(err).NotTo(HaveOccurred())
				twin := set.Records[0]
				twin.Journal = "edit-copy.jsonl"
				set.Records = append(set.Records, twin)
				set.Sources = append(set.Sources, Source{
					Journal: "edit-copy.jsonl", SHA256: set.Sources[0].SHA256, Records: 1,
				})
				g.Expect(Save(path, set)).To(Succeed())
			},
			refuse: true,
			remedy: "remove the other's records and its source line",
		},
		{
			name: "a journal recorded under a different hash whose content matches another source",
			setUp: func(g *WithT, journals, path string) {
				journal(g, journals, "x.jsonl", same(g))
				journal(g, journals, "y.jsonl", other(g))
				_, err := build(g, journals, path, false)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(os.Remove(filepath.Join(journals, "y.jsonl"))).To(Succeed())
				journal(g, journals, "x.jsonl", other(g))
			},
			refuse: true,
			remedy: "Rename it to y.jsonl",
		},
		{
			name: "genuinely new content, which is the case that must still be accepted",
			setUp: func(g *WithT, journals, path string) {
				journal(g, journals, "edit-aaa.jsonl", same(g))
				_, err := build(g, journals, path, false)
				g.Expect(err).NotTo(HaveOccurred())
				journal(g, journals, "edit-zzz.jsonl", other(g))
			},
			refuse: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			dir := t.TempDir()
			journals := filepath.Join(dir, "journals")
			g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
			path := filepath.Join(dir, "fixtures.jsonl")
			c.setUp(g, journals, path)

			set, err := Rebuild(journals, path, true)

			if !c.refuse {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(set.Sources).To(HaveLen(2), "the merge keeps both sources")
				g.Expect(set.Records).To(HaveLen(2))
				return
			}
			g.Expect(err).To(HaveOccurred(), "force does not override this one")
			g.Expect(err.Error()).To(ContainSubstring("twice"))
			g.Expect(err.Error()).To(ContainSubstring(c.remedy), "the remedy has to match the state, not merely name the other journal")
		})
	}
}

// TestTheRemedyDoesNotDependOnSortOrder pins what the incremental build of the
// present set broke. A copy whose name sorts before the journal it duplicates was
// told to rename itself over the recorded name, which is the one instruction that
// destroys evidence: if the recorded journal has since been rescored, renaming over
// it replaces the new content with the old.
func TestTheRemedyDoesNotDependOnSortOrder(t *testing.T) {
	cases := []struct {
		name     string
		recorded string
		copied   string
	}{
		{name: "the copy sorts after the recorded name", recorded: "edit-aaa.jsonl", copied: "edit-zzz.jsonl"},
		{name: "the copy sorts before it", recorded: "edit-zzz.jsonl", copied: "edit-aaa.jsonl"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			dir := t.TempDir()
			journals := filepath.Join(dir, "journals")
			g.Expect(os.Mkdir(journals, 0o750)).To(Succeed())
			path := filepath.Join(dir, "fixtures.jsonl")
			rows := []string{row(g, "sub_diff", "game", 1, "a", "b", "r1", "correct")}

			journal(g, journals, c.recorded, rows)
			_, err := build(g, journals, path, false)
			g.Expect(err).NotTo(HaveOccurred())
			journal(g, journals, c.copied, rows)

			_, err = Rebuild(journals, path, false)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring("Remove one of them"), "both files are on disk, whichever way the names sort")
			g.Expect(err.Error()).NotTo(ContainSubstring("Rename it to"), "renaming over a recorded journal is how its evidence is lost")
		})
	}
}
