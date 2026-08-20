package replay

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"ratchet/internal/dev/fixture"
)

// Disagreement is one record the applier judged differently from the harness.
type Disagreement struct {
	// Record is the fixture, so the reply can be read.
	Record fixture.Record
	// Got is what the applier decided.
	Got Outcome
}

// Counts is one set of tallies over some population of records.
type Counts struct {
	// Records is how many were replayed.
	Records int
	// Abstained is how many could not be judged from what was recorded, and are
	// counted out of the agreement rather than against it.
	Abstained int
	// Agreed is how many reached the harness's verdict.
	Agreed int
}

// Judged is how many records the agreement is computed over.
func (c Counts) Judged() int {
	return c.Records - c.Abstained
}

// Share is the agreement as a percentage of the judged records, and whether there
// were any to judge.
//
// The second return exists because zero is a real agreement rate and so is no data,
// and printing 0.00%% for both would overstate what was measured in a report whose
// purpose is not to.
func (c Counts) Share() (float64, bool) {
	if c.Judged() == 0 {
		return 0, false
	}
	return 100 * float64(c.Agreed) / float64(c.Judged()), true
}

// Tally counts one patch form twice, because the two counts answer different
// questions and reporting one invites it to be read as the other.
//
// All weights a reply by how often it was sent, which is what a real run would meet:
// an easy rename that four models answer identically really is four times as common
// as a hard one. Distinct counts each reply once, which is what coverage means: a
// common easy case cannot hide a rare disagreement behind it.
//
// Measured, they differ by about two and a half points, and All is the higher of the
// two. The duplicates sit in the records both sides get right, so quoting All alone
// would flatter the applier.
type Tally struct {
	// Form is the patch form.
	Form string
	// All is every record.
	All Counts
	// Distinct is one record per distinct reply and recorded verdict.
	Distinct Counts
}

// Report is a whole replay: the per-form counts and every disagreement.
type Report struct {
	// Tallies are the per-form counts, sorted by form.
	Tallies []Tally
	// Disagreements are the distinct disagreements, one per cause and reply rather
	// than one per record. The tallies count every record, so the two do not
	// reconcile by arithmetic: 68 records disagree and 67 of them are distinct.
	Disagreements []Disagreement
}

// Summarise replays the committed fixtures and prints the result.
//
// disagreements prints each one in full, which is what adjudicating needs and what
// reading the summary does not.
func Summarise(w io.Writer, disagreements bool) error {
	set, err := fixture.Load()
	if err != nil {
		return err
	}
	rep := Run(set.Records)
	if err := rep.Write(w); err != nil {
		return err
	}
	if !disagreements || len(rep.Disagreements) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return errors.Wrap(err, "printing the disagreements")
	}
	for _, d := range rep.Disagreements {
		verdict := "unjudged"
		if s, known := Explain(d); known {
			verdict = string(s.Side) + " was wrong: " + s.Why
		}
		_, err := fmt.Fprintf(
			w,
			"%s %s:%d  recorded=%s got=%s\n    original %q\n    wanted   %q\n    detail   %s\n    settled  %s\n    reply    %q\n",
			d.Record.Form,
			d.Record.Fixture,
			d.Record.Line,
			d.Record.Outcome,
			d.Got.Verdict,
			d.Record.Original,
			d.Record.Want,
			d.Got.Detail,
			verdict,
			d.Record.Reply,
		)
		if err != nil {
			return errors.Wrap(err, "printing a disagreement")
		}
	}
	return nil
}

// Run replays every record and collects the result.
func Run(records []fixture.Record) *Report {
	byForm := map[string]*Tally{}
	seen := map[string]struct{}{}
	rep := &Report{}
	for _, rec := range records {
		t := byForm[rec.Form]
		if t == nil {
			t = &Tally{
				Form: rec.Form,
			}
			byForm[rec.Form] = t
		}

		k := kindOf(rec)
		_, repeat := seen[k]
		seen[k] = struct{}{}

		got := Replay(rec)
		count(&t.All, rec, got)
		if !repeat {
			count(&t.Distinct, rec, got)
		}

		if got.Verdict == VerdictUnreplayable || string(got.Verdict) == rec.Outcome {
			continue
		}
		if repeat {
			continue
		}
		rep.Disagreements = append(rep.Disagreements, Disagreement{
			Record: rec,
			Got:    got,
		})
	}
	for _, t := range byForm {
		rep.Tallies = append(rep.Tallies, *t)
	}
	sort.Slice(rep.Tallies, func(i, j int) bool {
		return rep.Tallies[i].Form < rep.Tallies[j].Form
	})
	return rep
}

// count folds one replayed record into a set of tallies.
func count(c *Counts, rec fixture.Record, got Outcome) {
	c.Records++
	switch {
	case got.Verdict == VerdictUnreplayable:
		c.Abstained++
	case string(got.Verdict) == rec.Outcome:
		c.Agreed++
	}
}

// kindOf identifies a reply and the verdict it drew, so the same evidence arriving
// from several models or several runs is counted once where that is what is wanted.
func kindOf(rec fixture.Record) string {
	return strings.Join([]string{
		rec.Form,
		rec.Fixture,
		strconv.Itoa(rec.Line),
		rec.Reply,
		rec.Outcome,
	}, "\x00")
}

// Kinds groups the distinct disagreements by which pair of verdicts they are,
// because many replies failing the same way is one decision and not many.
func (r *Report) Kinds() map[string]int {
	out := map[string]int{}
	for _, d := range r.Disagreements {
		out[fmt.Sprintf("%s -> %s", d.Record.Outcome, d.Got.Verdict)]++
	}
	return out
}

// Write prints the report.
func (r *Report) Write(w io.Writer) error {
	header := "%-18s %8s %10s %10s %10s %10s\n"
	_, err := fmt.Fprintf(w, header, "form", "replies", "judged", "agreement", "distinct", "agreement")
	if err != nil {
		return err
	}
	for _, t := range r.Tallies {
		row := "%-18s %8d %10d %10s %10d %10s\n"
		_, err := fmt.Fprintf(w, row, t.Form, t.All.Records, t.All.Judged(), percent(t.All), t.Distinct.Judged(), percent(t.Distinct))
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w, "\nthe two agreements answer different questions: the first weights a reply by how\noften it was sent, the second counts each distinct reply once")
	if err != nil {
		return err
	}
	if len(r.Disagreements) == 0 {
		_, err := fmt.Fprintln(w, "\nno disagreements")
		return err
	}
	_, err = fmt.Fprintf(w, "\n%d disagreements, by kind:\n", len(r.Disagreements))
	if err != nil {
		return err
	}
	kinds := r.Kinds()
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		return kinds[names[i]] > kinds[names[j]]
	})
	for _, k := range names {
		if _, err := fmt.Fprintf(w, "  %-32s %d\n", k, kinds[k]); err != nil {
			return err
		}
	}
	return nil
}

// percent renders an agreement, or says there was nothing to judge.
func percent(c Counts) string {
	share, measured := c.Share()
	if !measured {
		return "none judged"
	}
	return fmt.Sprintf("%.2f%%", share)
}
