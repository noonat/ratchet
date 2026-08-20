package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// reHeader matches the section header: a path and a four-hex tag.
	reHeader = regexp.MustCompile(`^\[([^\]\r\n#]+)#([0-9A-Fa-f]{4})\]$`)
	// rePut matches `PUT N.=M:`, the whole-line form.
	rePut = regexp.MustCompile(`^PUT\s+(\d+)\s*\.=\s*(\d+)\s*:$`)
	// reSub matches `SUB N:`, the fragment form.
	reSub = regexp.MustCompile(`^SUB\s+(\d+)\s*:$`)
	// reFence matches a Markdown code fence, with or without a language tag.
	reFence = regexp.MustCompile("^`{3,}[a-zA-Z0-9_-]*$")
)

// Parse reads a reply and returns the patch it describes.
//
// Anything else is a Fault naming what is wrong. A parser that repairs silently
// turns a model's mistake into a file nobody reviewed.
//
// Blank lines and surrounding whitespace are tolerated.
func Parse(reply string) (*Patch, error) {
	lines := strings.Split(reply, "\n")
	p := &Patch{}
	var cur *Hunk
	// curLine is where the open hunk's header was, so a fault about an incomplete
	// hunk names that header rather than whatever line reading stopped on. The
	// number is what a corrective turn shows a model, so pointing it at the next
	// header sends the model to a line that is correct.
	curLine := 0
	// blankAt remembers a blank line seen inside an open hunk. Whether it matters
	// depends on what follows: a body row after it means the blank was a row whose
	// sigil went missing, which would silently shorten the replacement, while a
	// header or the end of the reply means it was only trailing space.
	blankAt := 0
	// fenceAt remembers a code fence seen inside an open hunk, for the same reason
	// as blankAt: whether it matters depends on whether a body row follows it.
	fenceAt := 0

	// close finishes the hunk being read, refusing one that never got both rows.
	// A hunk with only a `-` row states what to replace and not what to replace it
	// with, which is the one shape that looks complete and is not.
	close := func() error {
		if cur == nil {
			return nil
		}
		if len(cur.Old) == 0 || len(cur.New) == 0 {
			return faultAt(curLine, "a hunk needs a `-` row and a `+` row")
		}
		// A PUT names a range, so the `-` rows have to account for all of it. Two
		// rows under `PUT 10.=12:` is a reply that looks complete and describes a
		// file that does not exist.
		if cur.Kind == KindPut {
			want := cur.End - cur.Line + 1
			if len(cur.Old) != want {
				return faultAt(curLine, fmt.Sprintf("`PUT %d.=%d:` covers %d lines, so it needs %d `-` rows and has %d", cur.Line, cur.End, want, want, len(cur.Old)))
			}
		}
		// A SUB replaces one fragment inside one line, so more than one row either
		// side has no defined meaning and the applier could only guess.
		if cur.Kind == KindSub && (len(cur.Old) != 1 || len(cur.New) != 1) {
			return faultAt(curLine, "`SUB` replaces one fragment, so it takes one `-` row and one `+` row")
		}
		// Addresses are original line numbers, so hunks that overlap or run
		// backwards describe an order of application that the numbers deny.
		for _, done := range p.Hunks {
			if cur.Line <= done.End {
				return faultAt(curLine, fmt.Sprintf("this hunk starts at %d, which is not after the previous hunk ending at %d", cur.Line, done.End))
			}
		}
		p.Hunks = append(p.Hunks, *cur)
		cur, blankAt, fenceAt = nil, 0, 0
		return nil
	}

	for i, raw := range lines {
		n := i + 1
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			// Outside a hunk a blank line is spacing. Inside one, skipping it would
			// silently shorten the replacement, so it is refused. An empty line is
			// expressible: `+` on its own is a sigil with empty content.
			if cur != nil && blankAt == 0 {
				blankAt = n
			}
			continue
		}

		// A code fence around a reply is a wrapper, never content, and is skipped.
		// Measured over 3,789 recorded replies, 221 arrive fenced, and the harness that
		// scored them matched its patterns anywhere in the text, so every published
		// success rate for these forms already assumes the fence is ignored. Refusing
		// it would make this applier worse than the numbers that chose the format, on
		// 81 replies that were scored correct.
		//
		// Inside an open hunk it is remembered rather than skipped, for the same reason
		// as a blank line: dropping it would silently shorten the replacement. A model
		// replacing a Markdown line with a fenced block writes `+` rows whose content
		// is backticks, and a bare fence between two of those rows would otherwise
		// vanish and leave the file with an unterminated fence. None of the 221 fenced
		// replies has a body row after a fence, so remembering it costs nothing.
		if reFence.MatchString(strings.TrimSpace(line)) {
			if cur != nil && fenceAt == 0 {
				fenceAt = n
			}
			continue
		}

		if m := reHeader.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			if err := close(); err != nil {
				return nil, err
			}
			if p.Path != "" && (p.Path != m[1] || !strings.EqualFold(p.Tag, m[2])) {
				return nil, faultAt(n, "a reply addresses one file and one read, not several")
			}
			p.Path, p.Tag = m[1], strings.ToUpper(m[2])
			continue
		}

		trimmed := strings.TrimSpace(line)
		if m := rePut.FindStringSubmatch(trimmed); m != nil {
			if err := close(); err != nil {
				return nil, err
			}
			from, _ := strconv.Atoi(m[1])
			to, _ := strconv.Atoi(m[2])
			if from < 1 || to < from {
				return nil, faultAt(n, "line numbers run from 1 and the range cannot end before it starts")
			}
			cur, curLine = &Hunk{Kind: KindPut, Line: from, End: to}, n
			continue
		}
		if m := reSub.FindStringSubmatch(trimmed); m != nil {
			if err := close(); err != nil {
				return nil, err
			}
			at, _ := strconv.Atoi(m[1])
			if at < 1 {
				return nil, faultAt(n, "line numbers run from 1")
			}
			cur, curLine = &Hunk{Kind: KindSub, Line: at, End: at}, n
			continue
		}

		// Body rows. The sigil is the first character and the rest is content,
		// including a content dash or plus, which needs no escape.
		if cur == nil {
			// A line that is not a header and not a body row is usually prose, and
			// calling it a misplaced body row points the model at the wrong fix.
			if line[0] != '-' && line[0] != '+' {
				return nil, faultAt(n, "this is not a patch: expected a section header, then `PUT` or `SUB`, then `-` and `+` rows")
			}
			return nil, faultAt(n, "a body row before any `PUT` or `SUB` header")
		}
		if blankAt != 0 {
			return nil, faultAt(blankAt, "a blank line between body rows: an empty line is written as `+` alone")
		}
		if fenceAt != 0 {
			return nil, faultAt(fenceAt, "a code fence between body rows: a line of backticks in the replacement is written as `+` and then the backticks")
		}
		switch line[0] {
		case '-':
			if len(cur.New) > 0 {
				return nil, faultAt(n, "a `-` row after a `+` row")
			}
			cur.Old = append(cur.Old, line[1:])
		case '+':
			cur.New = append(cur.New, line[1:])
		default:
			return nil, faultAt(n, "a body row must start with `-` or `+`")
		}
	}

	if err := close(); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, fault("no section header, so no file and no read to check against")
	}
	if len(p.Hunks) == 0 {
		return nil, fault("no `PUT` or `SUB` header, so nothing to apply")
	}
	return p, nil
}

// AtMost refuses a patch carrying more hunks than were asked for.
//
// This is separate from Parse because the limit belongs to the request rather than
// to the grammar: a reply with eight hunks is well formed and, when one was asked
// for, edits seven lines nobody mentioned. Measured, that happens: asking two
// models for two hunks produced replies with 27, 57, 59, 68 and 71.
func (p *Patch) AtMost(n int) error {
	if len(p.Hunks) > n {
		return fault(fmt.Sprintf("the reply carries %d hunks and %d were asked for", len(p.Hunks), n))
	}
	return nil
}
