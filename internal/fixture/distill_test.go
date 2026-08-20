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
func build(t *testing.T, dir, path string, force bool) (*Set, error) {
	t.Helper()
	set, err := Rebuild(dir, path, force)
	if err != nil {
		return nil, err
	}
	NewWithT(t).Expect(Save(path, set)).To(Succeed())
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

	journal(t, journals, "a.jsonl", []string{
		row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(t, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
	})
	built, err := build(t, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(built.Records).To(HaveLen(2))
	before, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	// The fresh clone: the fixtures are committed, the journals are not.
	g.Expect(os.Remove(filepath.Join(journals, "a.jsonl"))).To(Succeed())
	rebuilt, err := build(t, journals, path, false)
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

	journal(t, journals, "b.jsonl", []string{row(t, "sub_diff", "game", 5, "e", "f", "r3", "correct")})
	journal(t, journals, "a.jsonl", []string{row(t, "put_diff_checked", "textwrap", 1, "a", "b", "r1", "refused")})

	_, err := build(t, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	pass1, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = build(t, journals, path, false)
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

	journal(t, journals, "a.jsonl", []string{
		row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(t, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(t, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())
	before, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())

	journal(t, journals, "a.jsonl", []string{
		row(t, "sub_diff", "game", 1, "a", "b", "r1", "applied_wrong"),
		row(t, "sub_diff", "game", 2, "c", "d", "r2", "correct"),
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

	journal(t, journals, "a.jsonl", []string{
		row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(t, "put_diff_checked", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(t, journals, path, false)
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

	journal(t, journals, "a.jsonl", []string{row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct")})
	_, err := build(t, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())

	journal(t, journals, "b.jsonl", []string{row(t, "put_diff_checked", "bullets", 7, "g", "h", "r2", "refused")})
	set, err := build(t, journals, path, false)

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

	for name := range Superseded {
		journal(t, journals, name, []string{
			row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
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

	journal(t, journals, "a.jsonl", []string{
		row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct"),
		row(t, "put_diff_checked", "game", 2, "c", "d", "r2", "correct"),
	})
	_, err := build(t, journals, path, false)
	g.Expect(err).NotTo(HaveOccurred())

	// a.jsonl now yields one record instead of two, and b.jsonl more than covers it
	restore := Forms
	Forms = map[string]struct{}{"sub_diff": {}}
	defer func() { Forms = restore }()
	journal(t, journals, "b.jsonl", []string{
		row(t, "sub_diff", "bullets", 7, "g", "h", "r3", "refused"),
		row(t, "sub_diff", "bullets", 8, "i", "j", "r4", "refused"),
		row(t, "sub_diff", "bullets", 9, "k", "l", "r5", "refused"),
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
		journal(t, journals, name, []string{row(t, "sub_diff", "game", 1, "a", "b", "r", "correct")})
	}

	_, err := Rebuild(journals, filepath.Join(dir, "fixtures.jsonl"), false)

	var refusal *Refusal
	g.Expect(errors.As(err, &refusal)).To(BeTrue())
	g.Expect(refusal.Reason).NotTo(BeEmpty())
}
