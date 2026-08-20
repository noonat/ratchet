package replay

import "strings"

// Side names which implementation was wrong about a disagreement.
type Side string

const (
	// SideApplier means this repo was wrong and was changed.
	SideApplier Side = "applier"
	// SideScorer means the harness was wrong. Where that is cheap to correct it was
	// corrected and the journal rescored; where it is not, the disagreement is left
	// standing with the reason.
	SideScorer Side = "scorer"
)

// Settled is a disagreement that has been adjudicated.
//
// Matching one is not the same as passing. It records that a person looked at the
// difference, decided which side was wrong, and either fixed that side or wrote down
// why it stays. A disagreement matching nothing here fails the gate, because the
// point of replaying is to hear about differences nobody has judged yet.
type Settled struct {
	// Got is the verdict this repo reaches.
	Got Verdict
	// Because is a substring of the applier's own detail, which is what identifies
	// the cause. Keyed on the cause rather than on a record, because 58 replies
	// failing one way is one decision.
	Because string
	// Side is who was wrong.
	Side Side
	// Why is the decision, in one line.
	Why string
	// Records is how many fixtures this covers, pinned.
	//
	// Without it the list records a cause and ratchets nothing: a change making three
	// hundred more replies fail an already-settled way would keep the gate green
	// while agreement fell from 97% to 80%. Adding a journal moves these numbers, and
	// moving them should mean reading them again.
	Records int
}

// settled is every adjudicated disagreement.
//
// The pattern behind all of them is one difference in kind. The harness scores by
// finding its patterns anywhere in a reply, so anything else in the text is invisible
// to it. This applier reads every line and refuses what it cannot account for. Where
// they disagree, the harness accepted a reply it had not fully read.
//
// Two of those were worth correcting at the source, and were: a code fence made this
// applier refuse 221 replies the harness had scored, and an old-row guard made the
// harness discard 97 correct replies whose content began with a dash. Both are fixed
// and the journal was rescored. What is left below is where the harness would have to
// stop being a pattern matcher, which would change every number it has published, so
// the difference is recorded instead.
var settled = []Settled{
	{
		Got:     VerdictMalformed,
		Because: "a body row must start with `-` or `+`",
		Side:    SideScorer,
		Why:     "the model wrote a correct patch and kept quoting, so bare rows follow it; the harness could not see them and this applier will not guess whether they are content",
		Records: 58,
	},
	{
		Got:     VerdictMalformed,
		Because: "which is not after the previous hunk ending at",
		Side:    SideScorer,
		Why:     "hunks out of order: the harness calls this applied_wrong because it models a tool that applies them all, and this one refuses instead",
		Records: 3,
	},
	{
		Got:     VerdictMalformed,
		Because: "takes one `-` row and one `+` row",
		Side:    SideScorer,
		Why:     "a `SUB` carrying several rows is refused here and was unparseable there, which is the same rejection under two names",
		Records: 3,
	},
	{
		Got:     VerdictMalformed,
		Because: "a `-` row after a `+` row",
		Side:    SideScorer,
		Why:     "rows out of order is refused here and was unparseable there, which is the same rejection under two names",
		Records: 1,
	},
	{
		Got:     VerdictRefused,
		Because: "Re-read the file, or send the edit again stating the line as it actually is",
		Side:    SideScorer,
		Why:     "the reply states old text belonging to another line; the harness called it unparseable and this applier names the line, which is the more useful of the two",
		Records: 2,
	},
}

// SettledList returns every decision, so a test can check each still applies.
func SettledList() []Settled {
	out := make([]Settled, len(settled))
	copy(out, settled)
	return out
}

// Explain returns the decision covering a disagreement, if one has been made.
func Explain(d Disagreement) (Settled, bool) {
	for _, s := range settled {
		if s.Got == d.Got.Verdict && strings.Contains(d.Got.Detail, s.Because) {
			return s, true
		}
	}
	return Settled{}, false
}

// Unsettled returns the disagreements nobody has judged yet.
func (r *Report) Unsettled() []Disagreement {
	var out []Disagreement
	for _, d := range r.Disagreements {
		if _, known := Explain(d); !known {
			out = append(out, d)
		}
	}
	return out
}
