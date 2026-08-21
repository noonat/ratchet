package fixture

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

const (
	// JournalDir is where the harness journals are copied, relative to the repository
	// root. Gitignored, so on a fresh clone it is empty.
	JournalDir = "journals"
	// Path is the committed fixtures file, relative to the repository root.
	Path = "testdata/fixtures.jsonl"
)

// Refresh rebuilds the committed fixtures and reports what went into them.
//
// The paths are fixed rather than arguments. They name files that belong to this
// repository, not to whatever directory the command was started from, and making them
// configurable would invite a rebuild that wrote somewhere else.
func Refresh(force bool, w io.Writer) error {
	set, err := Rebuild(JournalDir, Path, force)
	if err != nil {
		return err
	}
	if err := Save(Path, set); err != nil {
		return err
	}
	for _, s := range set.Sources {
		_, err := fmt.Fprintf(w, "  %-28s %6d records  %.12s\n", s.Journal, s.Records, s.SHA256)
		if err != nil {
			return errors.Wrap(err, "reporting the sources")
		}
	}
	_, err = fmt.Fprintf(w, "  %d records from %d journals\n", len(set.Records), len(set.Sources))
	return errors.Wrap(err, "reporting the totals")
}

// Load reads the committed fixtures.
func Load() (*Set, error) {
	f, err := os.Open(Path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening %s: run this from the repository root", Path)
	}
	defer f.Close()
	return Read(f)
}

// Rebuild regenerates the fixtures at path from the journals in dir.
//
// Additive, because the journals are gitignored. On a fresh clone dir is empty, and
// a target that rebuilt from whatever it found would replace the committed fixtures
// with nothing. Records whose journal is present are regenerated; records whose
// journal is absent are kept as they are.
//
// It refuses in two cases, both of which mean the fixtures are about to start
// asserting something different from what they assert now. force overrides both,
// and is the caller saying the change is intended.
func Rebuild(dir, path string, force bool) (*Set, error) {
	old, err := load(path)
	if err != nil {
		return nil, err
	}

	journals, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, errors.Wrapf(err, "listing journals in %s", dir)
	}

	recorded := map[string]Source{}
	for _, s := range old.Sources {
		recorded[s.Journal] = s
	}

	// Every name on disk, before checking any of them. Filled inside the loop it
	// would hold only the journals already visited, so a twin sorting later would
	// look absent and the refusal would say rename when it should say remove one.
	present := map[string]struct{}{}
	for _, journal := range journals {
		present[filepath.Base(journal)] = struct{}{}
	}

	next := &Set{}
	seenRun := map[string]string{}
	var runOrder []string
	for _, journal := range journals {
		name := filepath.Base(journal)

		if by, out := Superseded[name]; out && !force {
			return nil, refuse(
				"%s was superseded by %s, which reran the same cells. Keeping both counts every reply they share twice. Remove it, or re-run with FORCE=1",
				name,
				by,
			)
		}

		sum, err := hashFile(journal)
		if err != nil {
			return nil, err
		}
		// A second name on the same content, which no other check can see: the hashes
		// are unchanged, the counts only grow, and the totals only grow with them. It
		// runs before the rescored check so the refusal that names the other journal
		// wins over the one that only says this file moved.
		if other, ok := findTwin(name, sum, old.Sources, runOrder, seenRun); ok {
			return nil, refuseTwin(name, other, recorded, present, sum)
		}
		if was, seen := recorded[name]; seen && was.SHA256 != sum && !force {
			return nil, refuse(
				"%s has changed since the fixtures were built (%s, now %s). It was rescored or rerun, so the records built from it would assert something different. Re-run with FORCE=1 to accept that",
				name,
				shortHash(was.SHA256),
				shortHash(sum),
			)
		}

		records, err := extractFile(journal, name)
		if err != nil {
			return nil, err
		}
		if was, seen := recorded[name]; seen && len(records) < was.Records && !force {
			return nil, refuse(
				"%s yielded %d records where the fixtures record %d, with its hash unchanged, so the distiller now keeps less of it than it did. Re-run with FORCE=1 to accept that",
				name,
				len(records),
				was.Records,
			)
		}
		next.Sources = append(next.Sources, Source{
			Journal: name,
			SHA256:  sum,
			Records: len(records),
		})
		seenRun[name] = sum
		runOrder = append(runOrder, name)
		next.Records = append(next.Records, records...)
	}

	// Everything whose journal is not in dir is carried over untouched. This is the
	// fresh-clone case, and the one that would otherwise destroy the file.
	for _, s := range old.Sources {
		if _, here := present[s.Journal]; here {
			continue
		}
		next.Sources = append(next.Sources, s)
	}
	for _, rec := range old.Records {
		if _, here := present[rec.Journal]; !here {
			next.Records = append(next.Records, rec)
		}
	}

	// A backstop. The per-journal check above makes this unreachable through any
	// ordinary path, because the total is the sum of counts it has already compared.
	// What is left is a header whose recorded counts disagree with the records under
	// them, which is a hand-edited file, and it costs one comparison to notice.
	if len(next.Records) < len(old.Records) && !force {
		return nil, refuse(
			"rebuilding would drop %d of %d records. Re-run with FORCE=1 to accept it",
			len(old.Records)-len(next.Records),
			len(old.Records),
		)
	}
	return next, nil
}

// Save writes a set to path, through a temporary file so an interrupted write
// cannot leave a half-built fixtures file where a whole one was.
func Save(path string, s *Set) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fixtures-*")
	if err != nil {
		return errors.Wrapf(err, "creating a temporary file beside %s", path)
	}
	defer os.Remove(tmp.Name())

	if err := Write(tmp, s); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrap(err, "closing the temporary fixtures file")
	}
	// CreateTemp makes the file 0600, and a rename carries that over. A committed
	// file is 0644 in a fresh clone, so every rebuild would otherwise narrow it.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return errors.Wrapf(err, "setting the mode on %s", tmp.Name())
	}
	return errors.Wrapf(os.Rename(tmp.Name(), path), "replacing %s", path)
}

// load reads the fixtures that are already committed, treating a missing file as
// an empty set: the first run has nothing to merge with.
func load(path string) (*Set, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Set{}, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "opening %s", path)
	}
	defer f.Close()
	return Read(f)
}

// hashFile is Hash over a file on disk.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "opening %s", path)
	}
	defer f.Close()
	return Hash(f)
}

// extractFile is Extract over a file on disk.
func extractFile(path, name string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening %s", path)
	}
	defer f.Close()
	return Extract(name, f)
}

// shortHash trims a hash to something a person can compare in an error message.
func shortHash(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12]
}

// refuse constructs a Refusal carrying a stack from where the decision was made.
func refuse(format string, args ...any) error {
	return errors.WithStack(&Refusal{
		Reason: fmt.Sprintf(format, args...),
	})
}

// findTwin finds another name holding the same content as the journal being rebuilt.
//
// Two passes, because the twin can be either already in the fixtures or merely
// earlier in this run. The recorded pass catches a rename of, or a copy of, a
// journal the file already holds. The same-run pass catches a copy of a journal
// nothing has been rebuilt from yet, which every existing check waves through: its
// hash matches no record, its count grows, and so does the total.
//
// The lowest name wins when several match, so the message does not depend on map
// order. Under SHA-256 they are the same file, so which one is named is a matter of
// being repeatable rather than of being right.
func findTwin(name, sum string, sources []Source, runOrder []string, seenRun map[string]string) (string, bool) {
	found := ""
	for _, other := range sources {
		if other.Journal == name || other.SHA256 != sum {
			continue
		}
		if found == "" || other.Journal < found {
			found = other.Journal
		}
	}
	if found != "" {
		return found, true
	}
	for _, earlier := range runOrder {
		if seenRun[earlier] == sum {
			return earlier, true
		}
	}
	return "", false
}

// refuseTwin says which of the three states this is, and what to do about it.
//
// One fact selects the remedy: whether this name is already recorded under this very
// hash. If it is, the committed file holds both names over one hash and the fix is
// in the file. If it is not, the content is a copy, and the fix is on disk — remove
// one of two present files, or put the copy back under the recorded name. A name
// recorded under a different hash is the second case, not the first: the file then
// holds two names over two hashes, which is a copy rather than a doubled header.
//
// force is not consulted. The other rebuild refusals exist because the evidence
// under a named source genuinely changed and accepting that can be intended. Here
// the evidence did not change, it is doubled, and the remedy is to remove the
// duplicate rather than to record it twice.
func refuseTwin(name, other string, recorded map[string]Source, present map[string]struct{}, sum string) error {
	self, known := recorded[name]
	if known && self.SHA256 == sum {
		return refuse(
			"the fixtures already hold %s and %s under one hash, so every reply they share is counted twice. Keep one name: remove the other's records and its source line, and its file if it is still there, then rebuild",
			name,
			other,
		)
	}
	if _, alsoHere := present[other]; alsoHere {
		return refuse(
			"%s and %s are the same file, so keeping both counts every reply twice. Remove one of them, then rebuild",
			name,
			other,
		)
	}
	return refuse(
		"%s is a rename or a copy of %s, whose records the fixtures already hold, so keeping both counts every reply twice. Rename it to %s, or remove it, then rebuild",
		name,
		other,
		other,
	)
}
