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

// NewSnapshotForLines records a read. The lines given are the ones displayed,
// which is not always every line of the text: a window over a long file shows
// some of it and stamps a tag for all of it.
func NewSnapshotForLines(text string, lines []int) Snapshot {
	set := make(map[int]struct{}, len(lines))
	for _, n := range lines {
		set[n] = struct{}{}
	}
	return Snapshot{Tag: Tag(text), Text: text, Lines: set}
}

// NewSnapshot records a read that displayed the whole file, which is the common
// case and the one worth having a name for.
func NewSnapshot(text string) Snapshot {
	// Count on the text with line endings converted but trailing blanks left alone.
	// Normalize turns a whitespace-only last line into an empty one, which the rule
	// below then reads as a trailing newline and discards: a file whose last line
	// holds indentation and no newline would lose that line, and an edit to it would
	// be refused as a line nobody was shown.
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// A trailing newline produces a final empty element that was never a line on
	// screen, so it is not a line anyone can edit.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
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
