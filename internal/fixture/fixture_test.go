package fixture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// journalRow mirrors the part of a harness journal line that the distiller reads.
//
// Marshalled rather than built by hand. Every real reply is a multi-line patch, and
// hand-built JSON cannot hold one: the newlines and backticks that make a reply
// worth testing are the characters that break concatenation.
type journalRow struct {
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
	} `json:"verdict"`
}

// journal writes a journal file holding the given rows, and returns its path.
func journal(t *testing.T, dir, name string, rows []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	NewWithT(t).Expect(os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0o600)).To(Succeed())
	return path
}

// row is one journal line from the `edit` probe, holding the fields the distiller
// reads.
func row(t *testing.T, form, fixture string, line int, original, want, reply, outcome string) string {
	t.Helper()
	var r journalRow
	r.Probe = "edit"
	r.Variant = form
	r.Fixture = fixture
	r.Case.Line = line
	r.Case.Original = original
	r.Case.Want = want
	r.Reply = reply
	r.Verdict.Outcome = outcome
	return marshal(t, r)
}

// marshal renders a row, for a case that has to set a field row does not expose.
func marshal(t *testing.T, r journalRow) string {
	t.Helper()
	out, err := json.Marshal(r)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return string(out)
}

// patch is a reply as a model actually sends one: several lines, sigils, and a
// backtick-quoted path. Nothing built by string concatenation could hold it.
const patch = "[fixtures/game.js#3449]\nPUT 141.=141:\n-      if (Math.random() < 0.5) {\n+      if (Math.randomRenamed() < 0.5) {"

func TestExtractKeepsOnlyWhatCanBeReplayed(t *testing.T) {
	cases := []struct {
		name string
		line string
		kept bool
	}{
		{
			name: "a form this repo parses",
			line: row(t, "put_diff_checked", "game", 12, "a", "b", "reply", "correct"),
			kept: true,
		},
		{
			name: "the other form this repo parses",
			line: row(t, "sub_diff", "game", 12, "a", "b", "reply", "correct"),
			kept: true,
		},
		{
			name: "a form this repo does not parse",
			line: row(t, "sub_arrow", "game", 12, "a", "b", "reply", "correct"),
			kept: false,
		},
		{
			name: "a different probe, whose records are a different shape",
			line: marshal(t, otherProbe(t)),
			kept: false,
		},
		{
			name: "a reply as a model actually sends one, over several lines",
			line: row(t, "put_diff_checked", "game", 141, "      if (Math.random() < 0.5) {", "x", patch, "correct"),
			kept: true,
		},
		{
			name: "a reply that never arrived",
			line: row(t, "sub_diff", "game", 12, "a", "b", "", "failed"),
			kept: false,
		},
		{
			name: "a torn line, which an append-only writer can leave behind",
			line: `{"probe":"edit","variant":"sub_diff","case":{"line":1,"orig`,
			kept: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := Extract("j.jsonl", strings.NewReader(c.line+"\n"))

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(HaveLen(map[bool]int{true: 1, false: 0}[c.kept]))
		})
	}
}

// TestATornLineDoesNotLoseTheOnesBeforeIt is why Extract skips instead of failing.
// A journal is appended to by a process that can be killed mid-line, and one torn
// record is not a reason to refuse the tens of thousands ahead of it.
func TestATornLineDoesNotLoseTheOnesBeforeIt(t *testing.T) {
	g := NewWithT(t)
	body := row(t, "sub_diff", "game", 1, "a", "b", "r1", "correct") + "\n" +
		row(t, "sub_diff", "game", 2, "c", "d", "r2", "correct") + "\n" +
		`{"probe":"edit","variant":"sub_diff","cas`

	got, err := Extract("j.jsonl", strings.NewReader(body))

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(HaveLen(2))
}

// TestWriteIsReproducible is the property that lets the file be committed. Two runs
// over the same inputs have to produce the same bytes, or every rebuild is a diff.
func TestWriteIsReproducible(t *testing.T) {
	g := NewWithT(t)
	set := &Set{
		Sources: []Source{
			{Journal: "b.jsonl", SHA256: "bb", Records: 1},
			{Journal: "a.jsonl", SHA256: "aa", Records: 2},
		},
		Records: []Record{
			{Journal: "b.jsonl", Form: "sub_diff", Fixture: "game", Line: 9, Reply: "z"},
			{Journal: "a.jsonl", Form: "sub_diff", Fixture: "game", Line: 2, Reply: "b"},
			{Journal: "a.jsonl", Form: "sub_diff", Fixture: "game", Line: 2, Reply: "a"},
		},
	}

	var write1, write2 bytes.Buffer
	g.Expect(Write(&write1, set)).To(Succeed())
	g.Expect(Write(&write2, set)).To(Succeed())

	g.Expect(write2.String()).To(Equal(write1.String()))
	lines := strings.Split(strings.TrimRight(write1.String(), "\n"), "\n")
	g.Expect(lines).To(HaveLen(4), "a header and three records")
	g.Expect(lines[0]).To(ContainSubstring(`"journal":"a.jsonl"`), "sources are sorted")
	g.Expect(lines[1]).To(ContainSubstring(`"reply":"a"`), "records are sorted to the reply text")
}

// TestReadRoundTripsWrite carries a real multi-line reply through the file, which
// is the case that says the newlines inside a record cannot be mistaken for the
// newlines between records.
func TestReadRoundTripsWrite(t *testing.T) {
	g := NewWithT(t)
	set := &Set{
		Sources: []Source{
			{Journal: "a.jsonl", SHA256: "aa", Records: 1},
		},
		Records: []Record{
			{
				Journal:  "a.jsonl",
				Form:     "put_diff_checked",
				Fixture:  "game",
				Line:     141,
				Original: "      if (Math.random() < 0.5) {",
				Want:     "      if (Math.randomRenamed() < 0.5) {",
				Reply:    patch,
				Outcome:  "correct",
				Applied:  "      if (Math.randomRenamed() < 0.5) {",
			},
		},
	}

	var buf bytes.Buffer
	g.Expect(Write(&buf, set)).To(Succeed())
	got, err := Read(&buf)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Sources).To(Equal(set.Sources))
	g.Expect(got.Records).To(Equal(set.Records))
}

func TestReadTreatsAnEmptyFileAsAnEmptySet(t *testing.T) {
	g := NewWithT(t)

	got, err := Read(strings.NewReader(""))

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Sources).To(BeEmpty())
	g.Expect(got.Records).To(BeEmpty())
}

// otherProbe is a row from a probe whose records are a different shape.
func otherProbe(t *testing.T) journalRow {
	t.Helper()
	var r journalRow
	r.Probe = "multihunk"
	r.Variant = "put_diff_checked"
	r.Fixture = "game"
	r.Case.Line = 12
	r.Case.Original = "a"
	r.Reply = "reply"
	r.Verdict.Outcome = "correct"
	return r
}
