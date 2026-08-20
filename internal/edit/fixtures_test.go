// Package edit_test holds the check that runs the applier against every recorded
// reply. It is an external test package because the replay depends on this one, and
// a test inside `edit` could not import it.
package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/dev/fixture"
	"ratchet/internal/dev/replay"
)

// TestAgainstFixtures requires the applier to reach the harness's verdict on every
// recorded reply, or to disagree in a way somebody has already judged.
//
// This is the only test here that compares against an implementation nobody in this
// repo wrote. Every other test asserts what its author believed; this one asserts
// that two people, months apart and from the same measurements, built the same
// judgment. Where they differ, one of them is wrong, and the point is to be told.
//
// A new kind of disagreement fails. An old one is in internal/dev/replay's settled
// list, with the side that was wrong and the reason.
func TestAgainstFixtures(t *testing.T) {
	g := NewWithT(t)
	for _, d := range replayed(g).Unsettled() {
		t.Errorf(
			"unjudged disagreement: %s %s:%d recorded=%s got=%s\n  detail: %s\n  reply: %q",
			d.Record.Form,
			d.Record.Fixture,
			d.Record.Line,
			d.Record.Outcome,
			d.Got.Verdict,
			d.Got.Detail,
			d.Record.Reply,
		)
	}
}

// TestEverySettledDecisionCoversWhatItSays makes the settled list a ratchet rather
// than a note.
//
// Two ways it would otherwise rot. An entry matching nothing is a claim about the
// past nobody is checking, and the list should shrink as disagreements are resolved.
// An entry matching more than it did means a change made hundreds of replies fail an
// already-settled way, which is the failure a list keyed on causes cannot otherwise
// see: agreement could fall from 97% to 80% with every disagreement still
// "explained".
func TestEverySettledDecisionCoversWhatItSays(t *testing.T) {
	g := NewWithT(t)
	rep := replayed(g)

	covered := map[string]int{}
	for _, d := range rep.Disagreements {
		if s, known := replay.Explain(d); known {
			covered[s.Because]++
		}
	}

	for _, s := range replay.SettledList() {
		t.Run(s.Because, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(covered[s.Because]).
				To(Equal(s.Records), "this decision was settled over a different number of records than it covers now; read them again, then update the count or the decision")
		})
	}
}

// replayed reads the committed fixtures and runs them.
func replayed(g *WithT) *replay.Report {
	g.THelper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "fixtures.jsonl"))
	g.Expect(err).NotTo(HaveOccurred())
	defer f.Close()

	set, err := fixture.Read(f)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(set.Records).NotTo(BeEmpty(), "no fixtures to replay: has testdata moved?")
	return replay.Run(set.Records)
}
