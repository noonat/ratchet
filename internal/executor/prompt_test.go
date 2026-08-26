package executor

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/executor/tool"
	"ratchet/internal/patch"
)

// examples pulls the indented blocks out of the prompt, which is how it shows a
// read and the two edit forms.
var examples = regexp.MustCompile(`(?m)(?:^ {4}.*\n)+`)

// TestTheExamplesInThePromptParse is the defect this guards.
//
// A prompt that shows a form the parser rejects teaches a model to write
// something that will always be refused, and the refusal will not say the example
// was wrong. In the harness that preceded this repo a probe's prompt described a
// format its own scorer did not accept, and one model scored 0 of 50 on it.
func TestTheExamplesInThePromptParse(t *testing.T) {
	g := NewWithT(t)
	blocks := examples.FindAllString(System(), -1)
	g.Expect(blocks).To(HaveLen(3), "a read and the two edit forms")

	patches := blocks[1:]
	for _, block := range patches {
		text := dedent(block)
		t.Run(strings.SplitN(text, "\n", 3)[1], func(t *testing.T) {
			g := NewWithT(t)
			p, err := patch.Parse(text)
			g.Expect(err).NotTo(HaveOccurred(), "the prompt shows a patch the parser refuses")
			g.Expect(p.Path).To(Equal("path/to/file.go"))
			g.Expect(p.Tag).To(Equal("3449"))
			g.Expect(p.Hunks).To(HaveLen(1))
		})
	}
}

// TestThePromptNamesBothWaysToStop keeps the two terminal verbs in front of the
// model. Nothing else in the prompt tells it how to finish.
func TestThePromptNamesBothWaysToStop(t *testing.T) {
	g := NewWithT(t)
	g.Expect(System()).To(ContainSubstring(tool.NameDone))
	g.Expect(System()).To(ContainSubstring(tool.NameBlocked))
}

// TestTheTaskNamesTheFilesItMayTouch, because a model that has not been told
// which files are allowed spends a turn finding out from a refusal.
func TestTheTaskNamesTheFilesItMayTouch(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{
			name:  "one file",
			files: []string{"a.go"},
			want:  "a.go",
		},
		{
			name:  "several",
			files: []string{"a.go", "b.go"},
			want:  "a.go, b.go",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			got := Task(c.files, "rename the field")
			g.Expect(got).To(HavePrefix("rename the field"))
			g.Expect(got).To(ContainSubstring(c.want))
		})
	}

	t.Run("no files named", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(Task(nil, "rename the field")).To(Equal("rename the field"))
	})
}

// dedent removes the four spaces the prompt indents an example by, which is
// presentation rather than part of the form.
func dedent(block string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		out = append(out, strings.TrimPrefix(line, "    "))
	}
	return strings.Join(out, "\n")
}
