// Package edit resolves an anchor and applies a patch in memory.
//
// It does not normalize what a model wrote. A Unicode substitution table was
// specified for this stage on the grounds that it is "what a Q4 model damages when
// retyping code", and 19,055 recorded replies say otherwise: not one introduced a
// dash, quote or space variant into a payload. The 336 replies that contain one all
// copied it faithfully out of a file that already had it, and 289 of those scored
// correct, so converting to ASCII here would have silently changed 289 right answers
// into wrong ones. The damage that was measured is indentation, 30 replies in 119,
// and `patch.Reindent` is the repair for it.
//
// Nothing here writes to disk. Every stage that could reject an edit runs before
// any byte is produced, so a refusal cannot half-apply: the caller receives the
// file it already had, plus the attempt that was not made.
//
// A refusal returns three things, and SWE-agent's ablation is the argument for all
// three. Without the error the model misdiagnoses. Without its own attempt it sends
// the same edit again. Without the current file it edits against a memory four
// turns old.
package edit

import (
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
)

// Reason is why an edit was refused. The caller branches on it, so each cause the
// model can act on differently is its own value.
type Reason int

const (
	// ReasonNoRead means no read in this session served the path being edited.
	ReasonNoRead Reason = iota
	// ReasonMistranscribed means the anchor is wrong and the file is byte-for-byte what
	// was served, so the anchor is the only thing that can be wrong.
	ReasonMistranscribed
	// ReasonFileMoved means the file is not what was served. Whether the anchor was also
	// mistyped is unknowable, so this branch never names an anchor.
	ReasonFileMoved
	// ReasonLineNotShown means the read did not display a line the edit addresses.
	ReasonLineNotShown
	// ReasonOutOfRange means the edit addresses a line the file does not have.
	ReasonOutOfRange
	// ReasonOldMismatch means the text stated as being replaced is not the text there.
	ReasonOldMismatch
	// ReasonUnusable means the patch itself does not describe an edit: no hunks, a hunk
	// whose row count contradicts its range, or hunks that overlap or run backwards.
	//
	// One value rather than three, because the caller's answer to all of them is the
	// same and the message carries which it was. Parse refuses every shape of this,
	// so it is reachable only from a patch built in code, which the corpus replay
	// does.
	ReasonUnusable
)

// String names the reason for a log line or a test failure.
func (r Reason) String() string {
	switch r {
	case ReasonNoRead:
		return "no read issued an anchor for this file"
	case ReasonMistranscribed:
		return "the anchor was mistranscribed"
	case ReasonFileMoved:
		return "the file changed after the read"
	case ReasonLineNotShown:
		return "the read did not show this line"
	case ReasonOutOfRange:
		return "the file has no such line"
	case ReasonOldMismatch:
		return "the text being replaced is not what is there"
	case ReasonUnusable:
		return "the patch does not describe an edit"
	}
	return "unknown"
}

// Refusal is a rejected edit. It is an error because the caller has to be unable
// to ignore it, and it carries a Reason because the two anchor refusals differ in
// what they are allowed to say and a caller cannot tell them apart from prose.
type Refusal struct {
	// Reason is the cause, for the caller to branch on.
	Reason Reason
	// Message is what the model is shown.
	Message string
}

// Error makes Refusal an error.
func (r *Refusal) Error() string {
	return r.Message
}

// refuse constructs a Refusal carrying a stack from where it was constructed,
// which is the earliest point that knows which of the several causes applies.
func refuse(reason Reason, format string, args ...any) error {
	return errors.WithStack(&Refusal{
		Reason:  reason,
		Message: fmt.Sprintf(format, args...),
	})
}

// Result is what an edit produced. Now is always set, so a refusal still hands
// back the file the model should be looking at.
type Result struct {
	// Text is the file after the edit, set only when the edit was applied.
	Text string
	// Would is the text the edit would have produced, set only when the edit was
	// refused for what it said about content rather than for its anchor.
	//
	// An anchor refusal leaves it empty on purpose. Showing an attempt spliced into
	// a file the model has not read is the same silent wrong-line edit the anchor
	// exists to prevent, arriving through the rejection instead of through the edit.
	Would string
	// Now is the file as it stands, always set.
	Now string
}

// window renders the lines around a target, the way a read would show them, so a
// refusal that cannot name an anchor can still say what is there.
func window(text string, around, span int) string {
	lines := texts(open(text))
	first := around - span
	if first < 1 {
		first = 1
	}
	last := around + span
	if last > len(lines) {
		last = len(lines)
	}
	var b strings.Builder
	for n := first; n <= last; n++ {
		fmt.Fprintf(&b, "  %d: %s\n", n, lines[n-1])
	}
	return b.String()
}
