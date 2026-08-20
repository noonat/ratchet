package edit

import (
	"context"
	"strings"

	"ratchet/internal/anchor"
	"ratchet/internal/patch"
)

// Apply resolves the anchor, applies the hunks in memory, and returns the file that
// would result.
//
// The order matters: every check that can refuse runs before any output exists, so
// a refusal cannot leave a file half-edited. On refusal the result still carries the
// file as it stands, and carries the attempt itself when the refusal was about
// content rather than about the anchor.
func Apply(ctx context.Context, reads *anchor.Reads, p patch.Patch, current string, opts Options) (Result, error) {
	res := Result{
		Now: current,
	}
	if n, asked := len(p.Hunks), opts.hunks(); n > asked {
		return res, refuse(
			ReasonTooManyHunks,
			"The reply carries %d changes and %d was asked for. Send only the change that was requested.",
			n,
			asked,
		)
	}
	if _, err := Resolve(reads, p, current); err != nil {
		return res, err
	}

	rows := open(current)
	edited, err := splice(rows, p.Hunks, true)
	if err == nil {
		res.Text = render(edited)
		res.Diff = diff(rows, edited, p.Hunks)
		return res, nil
	}

	res.Would = attempted(rows, p.Hunks)
	return res, err
}

// attempted renders the edit the model asked for, ignoring whether the text it says
// it is replacing is really there. A refusal shows this back, so that a retry is not
// a re-send of the same edit.
//
// Empty when the hunks cannot be placed at all, because that is not an attempt at
// anything: there is no line for the replacement to land on. Only reached after the
// anchor resolved, so the file being spliced is the file the model read.
func attempted(rows []row, hunks []patch.Hunk) string {
	spliced, err := splice(rows, hunks, false)
	if err != nil {
		return ""
	}
	return render(spliced)
}

// splice replaces each hunk's lines, addressing the original line numbers. Parse
// has already refused hunks that overlap or run backwards, so one pass suffices.
//
// strict compares the text a hunk says it is replacing against the text that is
// there. Relaxing it produces the edit the model was asking for regardless, which is
// what a refusal shows back to it.
func splice(rows []row, hunks []patch.Hunk, strict bool) ([]row, error) {
	fallback := dominant(rows)
	out := make([]row, 0, len(rows))
	consumed := 0
	for _, h := range hunks {
		if h.Line < 1 || h.End > len(rows) {
			return nil, refuse(
				ReasonOutOfRange,
				"The file has %d lines, so lines %d to %d cannot be edited.",
				len(rows),
				h.Line,
				h.End,
			)
		}
		out = append(out, rows[consumed:h.Line-1]...)
		was := rows[h.Line-1 : h.End]
		switch h.Kind {
		case patch.KindSub:
			text, err := substitute(was[0].text, h, strict)
			if err != nil {
				return nil, err
			}
			out = append(out, row{
				text: text,
				end:  was[0].end,
			})
		default:
			if strict && !sameRows(texts(was), h.Old) {
				return nil, refuseMismatch(h, texts(was))
			}
			out = append(out, replace(h.New, was, fallback)...)
		}
		consumed = h.End
	}
	return append(out, rows[consumed:]...), nil
}

// refuseMismatch says what is there against what the hunk claimed, naming one line
// rather than a block the reader has to diff by eye.
//
// It always refuses. Returning nil would leave splice handing back an empty row list
// with no error, and Apply would render that as an empty file: a silent wipe rather
// than a refusal. usable already guarantees the row count matches the range, so the
// first branch and the last are unreachable today and stay because what they prevent
// is worse than what they cost.
func refuseMismatch(h patch.Hunk, was []string) error {
	if len(was) != len(h.Old) {
		return refuse(
			ReasonOldMismatch,
			"Lines %d to %d are %d lines, but the edit states %d of them.",
			h.Line,
			h.End,
			len(was),
			len(h.Old),
		)
	}
	for i := range was {
		if was[i] != h.Old[i] {
			return refuse(
				ReasonOldMismatch,
				"Line %d is `%s`, not `%s`. Re-read the file, or send the edit again stating the line as it actually is.",
				h.Line+i,
				was[i],
				h.Old[i],
			)
		}
	}
	return refuse(ReasonOldMismatch, "Lines %d to %d are not what the edit states.", h.Line, h.End)
}

// substitute replaces a fragment within one line.
//
// The fragment has to occur exactly once. Twice is ambiguous and the tool has no way
// to know which was meant, so it refuses rather than picking the first; not at all
// means the model is editing a line it has misremembered.
func substitute(row string, h patch.Hunk, strict bool) (string, error) {
	if n := strings.Count(row, h.Old[0]); n != 1 {
		if strict {
			return "", refuse(
				ReasonOldMismatch,
				"`%s` appears %d times on line %d, which reads `%s`. A fragment has to appear exactly once.",
				h.Old[0],
				n,
				h.Line,
				row,
			)
		}
		if n == 0 {
			return row, nil
		}
	}
	return strings.Replace(row, h.Old[0], h.New[0], 1), nil
}

// sameRows compares what a hunk says it is replacing against what is there.
func sameRows(was, stated []string) bool {
	if len(was) != len(stated) {
		return false
	}
	for i := range was {
		if was[i] != stated[i] {
			return false
		}
	}
	return true
}

// row is one line and the terminator that followed it. The terminator travels with
// the line so a file with mixed endings keeps them: rewriting every line's ending
// because one line was edited is the silent change to untouched code that this
// package refuses everywhere else.
type row struct {
	// text is the line without its terminator, so it compares equal to the row a
	// model wrote for it.
	text string
	// end is what followed it: "\n", "\r\n", or "" for an unterminated last line.
	end string
}

// open splits a file into rows, keeping each line's own terminator.
func open(text string) []row {
	var rows []row
	for len(text) > 0 {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			return append(rows, row{
				text: text,
			})
		}
		line, end := text[:i], "\n"
		if strings.HasSuffix(line, "\r") {
			line, end = line[:len(line)-1], "\r\n"
		}
		rows = append(rows, row{
			text: line,
			end:  end,
		})
		text = text[i+1:]
	}
	return rows
}

// render puts a file back together, terminator by terminator.
func render(rows []row) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.text)
		b.WriteString(r.end)
	}
	return b.String()
}

// dominant is the terminator a new line gets when there is no old line to inherit
// from. The most common one in the file, so an insertion into a CRLF file is CRLF and
// an insertion into an empty file is a plain newline.
func dominant(rows []row) string {
	crlf := 0
	lf := 0
	for _, r := range rows {
		switch r.end {
		case "\r\n":
			crlf++
		case "\n":
			lf++
		}
	}
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// replace renders a hunk's new text as rows.
//
// The last new row inherits the terminator of the last row it replaces, so replacing
// a file's unterminated last line does not invent a newline at the end. Every earlier
// row takes the file's dominant terminator, because a missing terminator is only
// legal on the final line and copying one inwards would join two lines together.
func replace(with []string, was []row, fallback string) []row {
	out := make([]row, 0, len(with))
	for i, text := range with {
		end := fallback
		if i == len(with)-1 && len(was) > 0 {
			end = was[len(was)-1].end
		}
		out = append(out, row{
			text: text,
			end:  end,
		})
	}
	return out
}

// texts is what window reads to quote a file back, sharing open's splitting so a
// quoted line matches what a read displayed.
func texts(rows []row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.text)
	}
	return out
}
