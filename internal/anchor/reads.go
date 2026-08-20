package anchor

import "path"

// Reads is what a session handed out: the latest snapshot per path.
//
// It exists for provenance, not for correctness. An edit whose anchor matches the
// file on disk exactly is still refused unless a read in this session issued that
// anchor, because a refusal message names the file's current state and a model can
// lift an identifier out of it and retry without ever seeing the new content. Every
// check downstream then passes, because the anchor is correct about a file the model
// has not read. Measured, a local model does this on roughly one refusal in fifteen,
// and the edits it produced would have deleted a constructor's `super(id);` and the
// declaration half of a two-line statement.
//
// Only the latest read of a path is kept. An earlier tag was issued legitimately,
// but if the file has changed since, honoring it writes over content the model was
// later shown differently; and if the file has not changed, the earlier tag is the
// same string as the current one.
type Reads struct {
	latest map[string]Snapshot
}

// NewReads returns an empty record of what a session has served.
func NewReads() *Reads {
	return &Reads{
		latest: map[string]Snapshot{},
	}
}

// Record stores what a read served for a path, replacing any earlier read of it.
func (r *Reads) Record(file string, s Snapshot) {
	r.latest[pathKey(file)] = s
}

// Snapshot returns what a read served for a path, and whether one did.
func (r *Reads) Snapshot(file string) (Snapshot, bool) {
	s, ok := r.latest[pathKey(file)]
	return s, ok
}

// pathKey is the path a snapshot is filed under.
//
// Cleaned, because `./src/a.ts` and `src//a.ts` name the file that `src/a.ts` names,
// and a lookup miss here produces a refusal telling the model to read a file it just
// read. Cleaning cannot reconcile an absolute path with a relative one, so the
// caller still has to file a snapshot under the path the read's own header renders.
func pathKey(file string) string {
	return path.Clean(file)
}
