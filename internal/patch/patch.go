// Package patch parses the reply a model sends when asked to change a line.
//
// Two forms are accepted, the two that measured best. Both name a line and give
// the text being replaced alongside its replacement, so the applier can check the
// old text before writing anything:
//
//	[dir/file.ts#1A2B]     the section header: path and the tag from the read
//	PUT 12.=12:            replace original lines 12 to 12
//	-const n = 1;          the line as it stands
//	+const n = 2;          the line as it should be
//
//	[dir/file.ts#1A2B]
//	SUB 12:                replace a fragment within original line 12
//	-const                 the fragment as it stands
//	+let                   the fragment as it should be
//
// A body row is its sigil followed by the content, verbatim. Content whose own
// first character is `-` or `+` needs no escape, because the sigil is always the
// first character of the row and everything after it is content. Nothing here
// doubles anything.
//
// That is worth stating because the measurement that chose this format called it a
// doubling rule while implementing plain prefixing: the prompt said "`- item`
// becomes `+- item`", which is a sigil and a literal dash rather than a doubled
// one, and the scorer accepted it. The name was wrong and the format was
// consistent. Adopting the name would have produced a parser that disagreed with
// every reply already recorded.
package patch

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// Kind is which of the two forms a hunk uses.
type Kind int

const (
	// Put replaces whole lines, Line through End inclusive.
	Put Kind = iota
	// Sub replaces a fragment within a single line.
	Sub
)

// String names the kind for an error message.
func (k Kind) String() string {
	if k == Sub {
		return "SUB"
	}
	return "PUT"
}

// Hunk is one change: where it applies, the text being replaced, and the
// replacement.
type Hunk struct {
	// Kind is Put or Sub.
	Kind Kind
	// Line is the first original line the hunk addresses, 1-indexed.
	Line int
	// End is the last original line, equal to Line for a single-line Put and for
	// every Sub.
	End int
	// Old is the text being replaced: whole lines for a Put, a fragment for a Sub.
	Old []string
	// New is the replacement, in the same shape as Old.
	New []string
}

// Patch is a reply: which file, which read it was written against, and the
// changes.
type Patch struct {
	// Path is the file named in the section header.
	Path string
	// Tag is the four-character tag copied from the read, which the applier
	// checks before touching anything.
	Tag string
	// Hunks are the changes, in the order they appeared.
	Hunks []Hunk
}

// Fault says why a reply could not be used. It is a distinct type because the
// applier's caller has to tell a reply it cannot parse from an edit it can parse
// and must refuse: the first is worth retrying with a corrective turn, the second
// is worth reporting to whoever asked for the edit.
type Fault struct {
	// Reason is what is wrong, in the words a model is shown.
	Reason string
	// Line is the 1-indexed line of the reply where the trouble is, or 0 when the
	// fault is about the reply as a whole.
	Line int
}

// faultAt constructs a Fault carrying a stack from where it was constructed.
//
// The stack belongs at the earliest origination point, and for an error made here
// that is this call rather than wherever it is finally handled. Eleven places in
// the parser return a Fault; by the time one reaches a caller, the frames that say
// which are gone unless they were captured here.
func faultAt(line int, reason string) error {
	return errors.WithStack(&Fault{
		Line:   line,
		Reason: reason,
	})
}

// fault constructs a Fault about the reply as a whole, with a stack.
func fault(reason string) error {
	return errors.WithStack(&Fault{
		Reason: reason,
	})
}

// Error makes Fault an error.
func (f *Fault) Error() string {
	if f.Line == 0 {
		return f.Reason
	}
	return fmt.Sprintf("reply line %d: %s", f.Line, f.Reason)
}

// Sigil is the character that opens a body row. Content follows it verbatim, so a
// payload whose own first character is a sigil needs no escape.
//
// The measurement that chose this format called that a doubling rule while
// implementing plain prefixing. Its prompt said "`- item` becomes `+- item`", which
// is a sigil followed by a literal dash rather than a doubled one, and its scorer
// accepted that. Adopting the name would have produced a parser that disagreed with
// every reply already recorded.
type Sigil byte

const (
	// Minus opens a row stating the text being replaced.
	Minus Sigil = '-'
	// Plus opens a row stating the replacement.
	Plus Sigil = '+'
)

// Row renders one body row: the sigil, then the content unchanged.
func Row(s Sigil, content string) string {
	return string(byte(s)) + content
}
