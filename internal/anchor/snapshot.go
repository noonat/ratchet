package anchor

import "strings"

// Snapshot is what a read served, recorded so a later edit can be checked against
// it rather than against the file on disk.
//
// Two fields exist for reasons that are easy to leave out and expensive to add
// back. Text makes the refusal branch decidable: on a tag mismatch the resolver has
// to know whether the file changed or the model mistyped, and those cases are
// indistinguishable from the anchor alone. Lines exists because a windowed or
// truncated read produces a tag for a file the model has only partly seen, and an
// edit outside that window is unreviewed by anyone.
type Snapshot struct {
	// Tag is the fingerprint stamped on the render.
	Tag string
	// Text is exactly what was served, so a mismatch can be attributed.
	Text string
	// Lines is the set of line numbers actually displayed, 1-indexed. A set, so
	// the value type carries no meaning to misread.
	Lines map[int]struct{}
}

// NewSnapshotForLines records a read that displayed part of a file.
//
// The lines given are the ones shown, which is not every line of the text: a window
// over a long file displays some of it and tags all of it, and an edit to a line
// nobody saw is unreviewed however well-formed its anchor is.
func NewSnapshotForLines(text string, lines []int) Snapshot {
	set := make(map[int]struct{}, len(lines))
	for _, n := range lines {
		set[n] = struct{}{}
	}
	return Snapshot{Tag: Tag(text), Text: text, Lines: set}
}

// Lines splits a file the way a read displays it, and the way an address counts.
//
// This is the one place that decides what a line is, because the two things that have
// to agree about it are the renderer that shows a model a file and the resolver that
// interprets an address afterwards. A second splitter written beside either of them
// is a second answer, and a listing that shows a line the applier will not accept is
// worse than no listing.
//
// A trailing newline terminates the last line rather than starting another, and CRLF
// is folded because a read does not display the carriage return.
func Lines(text string) []string {
	body := strings.ReplaceAll(text, "\r\n", "\n")
	if body == "" {
		return nil
	}
	out := strings.Split(body, "\n")
	if out[len(out)-1] == "" {
		return out[:len(out)-1]
	}
	return out
}

// NewSnapshot records a read that displayed the whole file.
//
// The common case, so it takes the plain name and NewSnapshotForLines says what makes
// the other one different.
func NewSnapshot(text string) Snapshot {
	// Counted on the raw text, not on Normalize's output. Normalize turns a
	// whitespace-only last line into an empty one, which Lines then reads as a
	// trailing newline and drops: a file whose last line holds indentation and no
	// newline would lose that line, and an edit to it would be refused as a line
	// nobody was shown.
	lines := Lines(text)
	nums := make([]int, 0, len(lines))
	for i := range lines {
		nums = append(nums, i+1)
	}
	return NewSnapshotForLines(text, nums)
}

// Shows reports whether the read displayed this line. An edit to a line it did not
// show is refused: an anchor proves the model saw a version of the file, not that
// it saw the line it is editing.
func (s Snapshot) Shows(line int) bool {
	_, shown := s.Lines[line]
	return shown
}
