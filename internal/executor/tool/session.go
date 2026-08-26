// Package tool holds the executor's calls: a read that records what it served, and
// an edit that writes its result to the file.
//
// It nests under the seat rather than sitting at the top level because the two seats'
// tool sets differ and neither is general. It is not in internal/edit because the
// applier may not reach a file. That package
// is fenced by an import allowlist admitting no filesystem, and the fence is what
// lets every refusal path be proven to touch nothing. Writing belongs to whatever
// calls the applier, so it lives here and depends on edit rather than the reverse.
package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"

	"ratchet/internal/anchor"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

// Session is what one run of the loop remembers.
//
// The reads outlive the call that served them, which is the whole point: an anchor
// carried by an edit three turns later has to resolve against the read that produced
// it, and a Reads built per call can only ever hold the file being edited.
type Session struct {
	root  string
	files map[string]struct{}
	reads *anchor.Reads
}

// NewSession starts a session over a directory, limited to the files an iteration
// names.
//
// An empty list means the whole root. The prompt tells a model which files it may
// touch, and until the list reached here that was a promise nothing kept: a model
// editing a different file under the root was told the edit applied.
func NewSession(root string, files ...string) *Session {
	allowed := map[string]struct{}{}
	for _, f := range files {
		allowed[filepath.Clean(f)] = struct{}{}
	}
	return &Session{root: root, files: allowed, reads: anchor.NewReads()}
}

// allow reports whether a path is one this iteration named.
//
// The wording comes from the architecture's error table, where this refusal is
// E_FILE_NOT_ALLOWED: the path is not in this iteration's files.
func (s *Session) allow(path string) error {
	if len(s.files) == 0 {
		return nil
	}
	if _, ok := s.files[filepath.Clean(path)]; ok {
		return nil
	}
	named := make([]string, 0, len(s.files))
	for f := range s.files {
		named = append(named, f)
	}
	sort.Strings(named)
	return errors.Newf("%s is not in this iteration's files: %s", path, strings.Join(named, ", "))
}

// Read renders a file the way a model is shown one: a tagged header, then numbered
// lines. It records what it served, so an edit some turns later can resolve against
// this read rather than against the file as it stands.
func (s *Session) Read(path string) (string, error) {
	if err := s.allow(path); err != nil {
		return "", err
	}
	text, err := s.load(path)
	if err != nil {
		return "", err
	}
	s.reads.Record(path, anchor.NewSnapshot(text))
	return listing(path, text), nil
}

// Edit applies a patch and, when it resolves, writes the result to the file.
//
// The write happens after every stage that could refuse, because the applier's own
// comment promises a refusal cannot half-apply and writing as each hunk resolved
// would make that false. The new text is then recorded as a read, so the tag it
// returns is one the next edit can carry: the file just changed, and the anchor the
// caller holds went stale the moment it did.
func (s *Session) Edit(ctx context.Context, p patch.Patch, opts edit.Options) (Result, error) {
	return s.apply(ctx, p, opts, true)
}

// Preview applies a patch and writes nothing, for a caller that wants to know what
// an edit would do. The loop has no use for it; a person at a shell does.
func (s *Session) Preview(ctx context.Context, p patch.Patch, opts edit.Options) (Result, error) {
	return s.apply(ctx, p, opts, false)
}

func (s *Session) apply(ctx context.Context, p patch.Patch, opts edit.Options, write bool) (Result, error) {
	// Checked before the applier runs, so a path outside the iteration is refused
	// on the same grounds whether the edit would have resolved or not.
	if err := s.allow(p.Path); err != nil {
		return Result{}, err
	}
	text, err := s.load(p.Path)
	if err != nil {
		return Result{}, err
	}
	res, err := edit.Apply(ctx, s.reads, p, text, opts)
	if err != nil || !write {
		return Result{Edit: res}, err
	}
	full, err := s.resolve(p.Path)
	if err != nil {
		return Result{Edit: res}, err
	}
	if err := replace(full, res.Text); err != nil {
		return Result{Edit: res}, err
	}
	// A whole-file snapshot, which is sound only because Read serves whole files.
	// Recording one after a windowed read would mark every line as displayed and
	// quietly retire the shown-lines check, so an edit to a line nobody saw would
	// resolve. Windowed reads have to decide what a write leaves visible.
	snap := anchor.NewSnapshot(res.Text)
	s.reads.Record(p.Path, snap)
	return Result{Edit: res, Tag: snap.Tag}, nil
}

// Result is what the applier decided, plus the tag of the file as it now stands.
//
// The tag lives here rather than on edit.Result because the applier never writes and
// so has no file to stamp. A caller needs it to make a second edit without a second
// read, which is the flow the provenance rule was measured against.
type Result struct {
	// Edit is what the applier decided: the text, the attempt, the file as it
	// stands, and the diff.
	Edit edit.Result
	// Tag is the fingerprint of the file after the edit, set only on success.
	Tag string
}

// listing is the shape a read takes on screen, and the shape an address refers to.
func listing(path, text string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s#%s]\n", path, anchor.Tag(text))
	for i, line := range anchor.Lines(text) {
		fmt.Fprintf(&b, "%d:%s\n", i+1, line)
	}
	return b.String()
}

// load reads a file under the session root.
func (s *Session) load(path string) (string, error) {
	full, err := s.resolve(path)
	if err != nil {
		return "", err
	}
	b, err := osReadFile(full)
	if err != nil {
		return "", errors.Wrapf(err, "reading %s", path)
	}
	return string(b), nil
}
