package edit

import (
	"ratchet/internal/anchor"
	"ratchet/internal/patch"
)

// Resolve decides whether a patch may be applied to a file, and returns the read
// it was written against.
//
// The refusal branches on whether the file moved, and only one branch may name a
// replacement anchor. The resolver can tell the two apart because it recorded what
// the read served; the anchor alone cannot distinguish them, so the decision belongs
// here and never to the model.
func Resolve(reads *anchor.Reads, p patch.Patch, current string) (anchor.Snapshot, error) {
	if err := usable(p); err != nil {
		return anchor.Snapshot{}, err
	}

	snap, issued := reads.Issued(p.Path)
	if !issued {
		return anchor.Snapshot{}, refuse(
			ReasonNoRead,
			"No read in this session showed `%s`, so it cannot be edited yet. Read it first. An anchor counts only if a read here issued it, even when it matches the file.",
			p.Path,
		)
	}

	// Whether the file moved is a question about the tag, not about the bytes. The
	// tag ignores trailing whitespace and line endings on purpose, so that an editor
	// that trims on save does not invalidate an anchor it never saw. Comparing bytes
	// here would refuse an edit to an untouched line because some other line lost a
	// space, and the refusal would print a window identical to what was displayed.
	moved := anchor.Tag(current) != snap.Tag

	// The file is still the one that was tagged, so a wrong anchor can only be a
	// wrong anchor. This is the one branch allowed to hand back the right one:
	// nothing moved, so the resolver knows what was meant.
	if p.Tag != snap.Tag && !moved {
		return anchor.Snapshot{}, refuse(
			ReasonMistranscribed,
			"The anchor for `%s` is `%s`, not `%s`. Nothing has changed since the read, so send the same edit again with the anchor copied exactly.",
			p.Path,
			snap.Tag,
			p.Tag,
		)
	}

	// The file is not what was tagged. Whether the anchor was also mistyped is
	// unknowable, and naming the file's current anchor here would tell the model to
	// edit content it has never seen. Show what is there and require a fresh read.
	if moved {
		return anchor.Snapshot{}, refuse(
			ReasonFileMoved,
			"`%s` changed after the read, so line %d is no longer what was shown. Read it again before editing.\n%s",
			p.Path,
			p.Hunks[0].Line,
			window(current, p.Hunks[0].Line, 2),
		)
	}

	if err := addressable(snap, p, current); err != nil {
		return anchor.Snapshot{}, err
	}
	return snap, nil
}

// usable refuses a patch that does not describe an edit.
//
// Parse rejects every shape below, so none of this is reachable from a model's
// reply. It is reachable from a patch built in code, which the corpus replay does,
// and the applier indexes rows and ranges directly: unchecked, each of these is a
// panic rather than a refusal.
func usable(p patch.Patch) error {
	if len(p.Hunks) == 0 {
		return refuse(ReasonUnusable, "The patch for `%s` contains no change to make.", p.Path)
	}
	prev := 0
	for i, h := range p.Hunks {
		if h.End < h.Line {
			return refuse(ReasonUnusable, "A hunk ends at line %d, before it starts at %d.", h.End, h.Line)
		}
		if h.Line <= prev {
			return refuse(
				ReasonUnusable,
				"Hunks have to be in order and cannot overlap: one starts at line %d after an earlier one covered line %d.",
				h.Line,
				prev,
			)
		}
		prev = h.End
		switch h.Kind {
		case patch.KindSub:
			if len(h.Old) != 1 || len(h.New) != 1 {
				return refuse(
					ReasonUnusable,
					"A `SUB` takes one line of old text and one of new; hunk %d has %d and %d.",
					i+1,
					len(h.Old),
					len(h.New),
				)
			}
		default:
			if want := h.End - h.Line + 1; len(h.Old) != want {
				return refuse(
					ReasonUnusable,
					"`PUT %d.=%d:` covers %d lines, so it needs %d lines of old text and has %d.",
					h.Line,
					h.End,
					want,
					want,
					len(h.Old),
				)
			}
		}
	}
	return nil
}

// addressable refuses an edit to a line the model cannot have seen, taking the more
// specific of the two reasons it can have.
//
// A line past the end of the file is out of range, not merely undisplayed. Both are
// true of it, and an error naming the wrong one points the model at the wrong fix:
// told the read was partial it reads again and finds the same file, where told the
// file is shorter than it thinks it corrects the address.
//
// Within the file, a line the read did not display is refused because an anchor
// proves the model saw a version of the file, not that it saw the line it is
// changing. Truncated reads, elided ranges and windowed output all produce anchors
// whose coverage is partial.
func addressable(snap anchor.Snapshot, p patch.Patch, current string) error {
	rows := len(open(current))
	for _, h := range p.Hunks {
		if h.Line < 1 || h.End > rows {
			return refuse(
				ReasonOutOfRange,
				"`%s` has %d lines, so lines %d to %d cannot be edited.",
				p.Path,
				rows,
				h.Line,
				h.End,
			)
		}
		for n := h.Line; n <= h.End; n++ {
			if !snap.Shows(n) {
				return refuse(
					ReasonLineNotShown,
					"The read of `%s` did not show line %d, so it cannot be edited. Read the lines to be changed.",
					p.Path,
					n,
				)
			}
		}
	}
	return nil
}
