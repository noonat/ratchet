package fixture

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

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

	next := &Set{}
	present := map[string]struct{}{}
	for _, journal := range journals {
		name := filepath.Base(journal)
		present[name] = struct{}{}

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
