package tool

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// osReadFile is separate so the session's own tests can be read without a detour
// through the filesystem package.
var osReadFile = os.ReadFile

// resolve turns a path the model wrote into one under the session root.
//
// A path that climbs out of the root is refused rather than cleaned into place. The
// alternative is a tool that edits whatever a mistyped address happens to name, and
// the root is the only statement anyone has made about what this session may touch.
func (s *Session) resolve(path string) (string, error) {
	full := filepath.Join(s.root, path)
	if err := s.contains(full); err != nil {
		return "", err
	}
	// Links are resolved and checked again, because a lexical check passes for a
	// link inside the root that points anywhere at all. Only the target's location
	// says what a read would serve or a write would reach.
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		if os.IsNotExist(err) {
			return full, nil
		}
		return "", errors.Wrapf(err, "resolving %s", path)
	}
	if err := s.contains(real); err != nil {
		return "", err
	}
	return real, nil
}

// contains reports whether a resolved path is under the session root.
func (s *Session) contains(full string) error {
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		root = s.root
	}
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.Newf("%s is outside the session root", full)
	}
	return nil
}

// replace writes text over path, keeping the mode of what it replaced.
//
// A temporary file beside the target and a rename, so a failed write leaves the
// original rather than a truncated file. The mode comes from what is being replaced
// because CreateTemp makes 0600 and a rename carries that over, and a source file
// belongs to the repository: every edit would otherwise leave it at 0600.
func replace(path, text string) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "reading the mode of %s", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ratchet-*")
	if err != nil {
		return errors.Wrapf(err, "creating a temporary file beside %s", path)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return errors.Wrapf(err, "writing %s", tmp.Name())
	}
	if err := tmp.Close(); err != nil {
		return errors.Wrapf(err, "closing %s", tmp.Name())
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return errors.Wrapf(err, "setting the mode on %s", tmp.Name())
	}
	return errors.Wrapf(os.Rename(tmp.Name(), path), "replacing %s", path)
}
