// Package fixture distills the measurement harness's journals into the replay
// fixtures committed under testdata.
//
// A journal is 8 to 76MB and almost all of it is the prompt text sent to the
// model, which the applier never reads. What it reads is one line's original
// text, the reply that proposed a change to it, the text that change should have
// produced, and the verdict the harness recorded. That is what is kept.
//
// The journals are gitignored, so a fresh clone has the fixtures and not their
// sources. Everything here is shaped by that: rebuilding merges rather than
// overwrites, refuses to shrink, and writes sorted so two runs over the same
// inputs produce the same bytes.
package fixture

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	// scanBuffer is the per-line buffer a journal scanner starts with.
	scanBuffer = 64 * 1024
	// scanLimit is the longest line accepted. bufio.Scanner stops with an error past
	// its limit rather than skipping the line, so too low a limit abandons the rest
	// of the journal.
	//
	// A journal line holds the whole prompt, which is the read window plus the
	// reply. The longest across the journals in hand is 22,876 bytes, from a
	// 400-line window, so this is generous on purpose: a wider window scales that
	// number, and the cost of a limit set high is nothing until a line approaches it.
	scanLimit = 4 * 1024 * 1024
)

// Forms are the patch forms this repo parses, and the only ones worth keeping.
//
// The harness measured eleven. A reply in one of the other nine would be refused
// for its syntax, which says nothing about the applier, so storing it would add
// bulk and a guaranteed disagreement.
var Forms = map[string]struct{}{
	"put_diff_checked": {},
	"sub_diff":         {},
}

// Refusal is a rebuild declined on purpose, as against an error the distiller ran
// into. The distinction is what a person is shown: a refusal is a message written
// for them, and a fault is worth its stack.
type Refusal struct {
	// Reason is the message, already phrased for whoever ran the target.
	Reason string
}

// Error makes Refusal an error.
func (r *Refusal) Error() string {
	return r.Reason
}

// Superseded names journals kept out because another journal reran the same cells.
// Copying one in doubles the weight of every reply it shares with its replacement,
// and the guards cannot see it: the hashes are unchanged and the count grows.
//
// journals/README.md says to leave these out. This is the same rule, enforced,
// because the obvious way to follow that README is a glob that picks them up.
var Superseded = map[string]string{
	"edit-think-low.jsonl": "edit-think-low-4k.jsonl",
}

// Record is one reply, reduced to what replaying it needs.
type Record struct {
	// Journal is the file this came from, so a disagreement can be traced back to
	// the run that produced it.
	Journal string `json:"journal"`
	// Form is the patch form the reply was asked for.
	Form string `json:"form"`
	// Fixture names the file that was edited. It is kept for the language, which
	// decides whether re-indentation may run, and which nothing else here carries.
	Fixture string `json:"fixture"`
	// Line is the line the edit addresses, numbered from the original file.
	Line int `json:"line"`
	// Original is that line as the file held it.
	Original string `json:"original"`
	// Want is the line the edit should have produced.
	Want string `json:"want"`
	// Reply is what the model sent, verbatim.
	Reply string `json:"reply"`
	// Outcome is the verdict the harness recorded: correct, applied_wrong, refused
	// or malformed.
	//
	// Not failed. A failed attempt is one that never produced a reply to replay, and
	// a record with no reply text is skipped, so the class cannot reach here. In the
	// journals distilled so far that is every failed attempt, 230 of them.
	Outcome string `json:"outcome"`
	// Detail is the symptom behind the outcome, when there was one.
	Detail string `json:"detail,omitempty"`
	// Applied is the line the harness produced from this reply, when it produced
	// one. A replay that applies the same reply has to reach the same text.
	Applied string `json:"applied,omitempty"`
}

// Source is one journal a fixtures file was built from.
type Source struct {
	// Journal is the file name, without a directory: the path it was read from
	// belongs to whoever ran the target, not to the fixtures.
	Journal string `json:"journal"`
	// SHA256 is the journal's hash, so a journal that was rescored is detected
	// rather than silently changing what the fixtures assert.
	SHA256 string `json:"sha256"`
	// Records is how many records this journal contributed.
	Records int `json:"records"`
}

// Set is a fixtures file: the journals it came from, then the records.
type Set struct {
	// Sources are the journals, sorted by name.
	Sources []Source `json:"sources"`
	// Records are the distilled replies, sorted so the file is reproducible.
	Records []Record `json:"-"`
}

// Extract reads one journal and returns the records worth keeping.
//
// A line that does not parse is skipped rather than fatal. A journal is written
// by an append-only writer that can be killed mid-line, and a torn last line is
// not a reason to refuse the 40,000 before it.
func Extract(journal string, r io.Reader) ([]Record, error) {
	var out []Record
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, scanBuffer), scanLimit)
	for in.Scan() {
		var row struct {
			Probe   string `json:"probe"`
			Variant string `json:"variant"`
			Fixture string `json:"fixture"`
			Case    struct {
				Line     int    `json:"line"`
				Original string `json:"original"`
				Want     string `json:"want"`
			} `json:"case"`
			Reply   string `json:"reply"`
			Verdict struct {
				Outcome string `json:"outcome"`
				Detail  string `json:"detail"`
				Applied string `json:"applied"`
			} `json:"verdict"`
		}
		if err := json.Unmarshal(in.Bytes(), &row); err != nil {
			continue
		}
		if row.Probe != "edit" {
			continue
		}
		if _, keep := Forms[row.Variant]; !keep {
			continue
		}
		if row.Case.Original == "" || row.Reply == "" {
			continue
		}
		out = append(out, Record{
			Journal:  journal,
			Form:     row.Variant,
			Fixture:  row.Fixture,
			Line:     row.Case.Line,
			Original: row.Case.Original,
			Want:     row.Case.Want,
			Reply:    row.Reply,
			Outcome:  row.Verdict.Outcome,
			Detail:   row.Verdict.Detail,
			Applied:  row.Verdict.Applied,
		})
	}
	if err := in.Err(); err != nil {
		return nil, errors.Wrapf(err, "reading journal %s", journal)
	}
	return out, nil
}

// Hash is a journal's SHA-256, hex encoded.
func Hash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", errors.Wrap(err, "hashing a journal")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Read parses a fixtures file.
func Read(r io.Reader) (*Set, error) {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, scanBuffer), scanLimit)
	if !in.Scan() {
		return &Set{}, nil
	}
	set := &Set{}
	if err := json.Unmarshal(in.Bytes(), set); err != nil {
		return nil, errors.Wrap(err, "reading the fixtures header")
	}
	for in.Scan() {
		if strings.TrimSpace(in.Text()) == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal(in.Bytes(), &rec); err != nil {
			return nil, errors.Wrap(err, "reading a fixture record")
		}
		set.Records = append(set.Records, rec)
	}
	return set, errors.Wrap(in.Err(), "reading the fixtures file")
}

// Write emits a fixtures file: the header, then one record per line, sorted.
func Write(w io.Writer, s *Set) error {
	out := bufio.NewWriter(w)
	sortSet(s)
	header, err := json.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "encoding the fixtures header")
	}
	if _, err := fmt.Fprintf(out, "%s\n", header); err != nil {
		return errors.Wrap(err, "writing the fixtures header")
	}
	for i := range s.Records {
		line, err := json.Marshal(&s.Records[i])
		if err != nil {
			return errors.Wrap(err, "encoding a fixture record")
		}
		if _, err := fmt.Fprintf(out, "%s\n", line); err != nil {
			return errors.Wrap(err, "writing a fixture record")
		}
	}
	return errors.Wrap(out.Flush(), "flushing the fixtures file")
}

// sortSet orders sources and records so the same inputs produce the same bytes.
//
// The key runs to the reply text because one case is answered by several models,
// and the model is not kept. Two byte-identical replies to one case are then
// indistinguishable, which is correct: they are the same evidence twice.
func sortSet(s *Set) {
	sort.Slice(s.Sources, func(i, j int) bool {
		return s.Sources[i].Journal < s.Sources[j].Journal
	})
	sort.Slice(s.Records, func(i, j int) bool {
		a, b := &s.Records[i], &s.Records[j]
		switch {
		case a.Journal != b.Journal:
			return a.Journal < b.Journal
		case a.Form != b.Form:
			return a.Form < b.Form
		case a.Fixture != b.Fixture:
			return a.Fixture < b.Fixture
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Reply != b.Reply:
			return a.Reply < b.Reply
		default:
			return a.Outcome < b.Outcome
		}
	})
}
