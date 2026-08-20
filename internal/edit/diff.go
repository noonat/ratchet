package edit

import (
	"fmt"
	"strings"

	"ratchet/internal/patch"
)

// Context is how many unchanged lines surround a change in the diff.
const Context = 3

// diff renders what changed, in the shape a person reads a patch in.
//
// It exists because the caller has to be able to show the change without diffing the
// files again, and because a tool that reports what it did in the same notation the
// model was asked to write in reduces the number of formats in play by one.
//
// Built from the hunks rather than by comparing the two texts. Comparing them means
// finding where they stop matching and where they start again, which for one change
// is the same answer and for two is everything between them: two one-line edits
// eighteen lines apart reported thirty-four lines as changed, thirty of them
// identical. The hunks say exactly which lines moved, so nothing has to be inferred.
func diff(was, now []row, hunks []patch.Hunk) string {
	var b strings.Builder
	for _, blk := range blocks(was, hunks) {
		write(&b, was, now, blk)
	}
	return b.String()
}

// placed is one hunk, with the lines it replaces and the lines that replace them.
type placed struct {
	// oldFrom and oldTo are the original lines, 1-indexed inclusive.
	oldFrom, oldTo int
	// newFrom and newTo are their replacements in the result.
	newFrom, newTo int
}

// block is one `@@` section: the changes it covers and the context around them.
type block struct {
	// changes are in order and do not overlap, so the lines between two of them are
	// unchanged and print as context.
	changes []placed
	// from and to bound the block in the original, context included.
	from, to int
}

// blocks places each hunk in both files and groups the ones close enough to share
// context.
//
// Grouping matters because two changes four lines apart would otherwise print the
// lines between them twice, once as one block's trailing context and once as the
// next block's leading context. A diff that shows the same line twice reads as more
// change than happened.
func blocks(was []row, hunks []patch.Hunk) []block {
	var out []block
	shift := 0
	for _, h := range hunks {
		p := placed{
			oldFrom: h.Line,
			oldTo:   h.End,
			newFrom: h.Line + shift,
			newTo:   h.Line + shift + len(h.New) - 1,
		}
		shift += len(h.New) - (h.End - h.Line + 1)

		from := max(1, p.oldFrom-Context)
		to := min(len(was), p.oldTo+Context)
		if n := len(out); n > 0 && from <= out[n-1].to+1 {
			last := &out[n-1]
			// Touching the previous change with no unchanged line between them: one
			// change, so its removals group before its additions the way a diff reads.
			if prev := len(last.changes) - 1; last.changes[prev].oldTo+1 == p.oldFrom {
				last.changes[prev].oldTo = p.oldTo
				last.changes[prev].newTo = p.newTo
			} else {
				last.changes = append(last.changes, p)
			}
			last.to = to
			continue
		}
		out = append(out, block{
			changes: []placed{p},
			from:    from,
			to:      to,
		})
	}
	return out
}

// write emits one `@@` section, alternating context and change.
func write(b *strings.Builder, was, now []row, blk block) {
	first := blk.changes[0]
	oldCount := blk.to - blk.from + 1
	newCount := oldCount
	for _, c := range blk.changes {
		newCount += (c.newTo - c.newFrom + 1) - (c.oldTo - c.oldFrom + 1)
	}
	newFrom := first.newFrom - (first.oldFrom - blk.from)
	fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@\n", blk.from, oldCount, newFrom, newCount)

	at := blk.from
	for _, c := range blk.changes {
		for ; at < c.oldFrom; at++ {
			fmt.Fprintf(b, " %s\n", was[at-1].text)
		}
		for i := c.oldFrom; i <= c.oldTo; i++ {
			fmt.Fprintf(b, "-%s\n", was[i-1].text)
		}
		for i := c.newFrom; i <= c.newTo; i++ {
			fmt.Fprintf(b, "+%s\n", now[i-1].text)
		}
		at = c.oldTo + 1
	}
	for ; at <= blk.to; at++ {
		fmt.Fprintf(b, " %s\n", was[at-1].text)
	}
}
