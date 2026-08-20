// Package replay runs recorded replies through the applier and says whether it
// reaches the verdict the measurement harness reached.
//
// It exists as its own package because it depends on both halves: the applier under
// test, and the fixtures that feed it. Putting it in either would make that one
// depend on the other, and the applier must not know about the plumbing that checks
// it.
//
// What a disagreement means is the point of the exercise. Two implementations judged
// the same reply, written months apart from the same measurements, and where they
// differ exactly one of them is wrong. The report names the difference; a person
// decides which side to fix.
package replay

import (
	"fmt"

	"ratchet/internal/anchor"
	"ratchet/internal/dev/fixture"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

// Verdict is what happened, named as the harness named it so the two are comparable
// without a translation table sitting between them.
type Verdict string

const (
	// VerdictCorrect means the addressed line came out as the edit intended.
	VerdictCorrect Verdict = "correct"
	// VerdictAppliedWrong means an edit was made and the line is not what was wanted.
	VerdictAppliedWrong Verdict = "applied_wrong"
	// VerdictRefused means the applier declined and changed nothing.
	VerdictRefused Verdict = "refused"
	// VerdictMalformed means the reply could not be parsed at all.
	VerdictMalformed Verdict = "malformed"
	// VerdictUnreplayable means this reply cannot be judged from what was recorded.
	//
	// The harness recorded one line. A reply that addresses a different one has to be
	// applied to content nothing here has, so the applier would be comparing against
	// filler and the result would say nothing about either side. The harness judged
	// these against the real file and its verdict stands; this replay abstains.
	VerdictUnreplayable Verdict = "unreplayable"
)

// Outcome is one replayed reply.
type Outcome struct {
	// Verdict is the comparable classification.
	Verdict Verdict
	// Detail says why, and is what a person reads when adjudicating.
	Detail string
}

// Requested is how many hunks the harness asked for. Every fixture comes from a
// probe that named one line, and a reply carrying more is well formed while editing
// lines nobody mentioned.
const Requested = 1

// Tail is how many filler lines follow the addressed one, so that replacing it with
// several lines has somewhere to go and the file does not end at the edit.
const Tail = 5

// Replay applies one recorded reply and classifies the result.
//
// The harness recorded a line and not a file: its number, its text before the edit,
// and the text the edit should have produced. So the reply is applied to a file built
// for it, with that line at that number and inert filler around it.
//
// Two substitutions make that possible, and both are stated rather than hidden. The
// anchor is replaced with the built file's own, because a tag names the file it was
// made from and cannot survive being moved to another. Nothing measurable is lost:
// of the 4,099 recorded attempts in these two forms, 3,788 got as far as writing a
// section header and all 3,788 carried the tag they were served, so the check being
// skipped is one that never fired. The read covers the whole file, because the harness
// always displayed the whole window.
//
// So this exercises the parser, the hunk limit, the old-line check and the splice. It
// cannot reach the four refusals that need something only an environment can do: an
// unread path, a moved file, a windowed read, or an anchor the tool never issued.
// Those are covered in internal/edit's own tests, where the setup is visible.
func Replay(rec fixture.Record) Outcome {
	p, err := patch.Parse(rec.Reply)
	if err != nil {
		return Outcome{
			Verdict: VerdictMalformed,
			Detail:  err.Error(),
		}
	}
	if err := p.AtMost(Requested); err != nil {
		return Outcome{
			Verdict: VerdictAppliedWrong,
			Detail:  err.Error(),
		}
	}

	for _, h := range p.Hunks {
		if h.Line != rec.Line || h.End != rec.Line {
			return Outcome{
				Verdict: VerdictUnreplayable,
				Detail:  fmt.Sprintf("addresses lines %d to %d, and only line %d was recorded", h.Line, h.End, rec.Line),
			}
		}
	}

	file := Build(rec.Line, rec.Original)
	snap := anchor.NewSnapshot(file)
	reads := anchor.NewReads()
	reads.Record(p.Path, snap)
	p.Tag = snap.Tag

	res, err := edit.Apply(reads, *p, file)
	if err != nil {
		return Outcome{
			Verdict: VerdictRefused,
			Detail:  err.Error(),
		}
	}

	got, ok := Line(res.Text, rec.Line)
	if !ok {
		return Outcome{
			Verdict: VerdictAppliedWrong,
			Detail:  fmt.Sprintf("the edit left the file with fewer than %d lines", rec.Line),
		}
	}
	if got != rec.Want {
		return Outcome{
			Verdict: VerdictAppliedWrong,
			Detail:  fmt.Sprintf("line %d became %q, wanted %q", rec.Line, got, rec.Want),
		}
	}
	return Outcome{
		Verdict: VerdictCorrect,
	}
}

// Build makes a file holding the recorded line at its recorded number.
//
// The filler is a single space. It cannot be mistaken for content in any language the
// fixtures cover, and nothing reads it: only the addressed line is compared, and the
// anchor is recomputed over whatever this produces.
func Build(line int, original string) string {
	out := make([]byte, 0, len(original)+2*(line+Tail))
	for n := 1; n < line; n++ {
		out = append(out, ' ', '\n')
	}
	out = append(out, original...)
	out = append(out, '\n')
	for n := 0; n < Tail; n++ {
		out = append(out, ' ', '\n')
	}
	return string(out)
}

// Line returns one line of a file by number, and whether the file has it.
//
// A trailing newline terminates the last line rather than starting another, so
// Line("a\n", 2) reports that there is no line 2. Counting the empty string after it
// would make every file appear to have one more line than it shows.
func Line(text string, n int) (string, bool) {
	if n < 1 {
		return "", false
	}
	at := 1
	for start := 0; start < len(text); {
		end := start
		for end < len(text) && text[end] != '\n' {
			end++
		}
		if at == n {
			return text[start:end], true
		}
		at++
		start = end + 1
	}
	return "", false
}
