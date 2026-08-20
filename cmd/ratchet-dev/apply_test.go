package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v3"

	"ratchet/internal/anchor"
)

// TestRenderIsWhatAPatchIsWrittenAgainst pins the listing, because a patch carries the
// anchor this prints and the two have to agree exactly.
func TestRenderIsWhatAPatchIsWrittenAgainst(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "an ordinary file",
			text: "a\nb\n",
			want: []string{"1:a", "2:b"},
		},
		{
			name: "no trailing newline",
			text: "a\nb",
			want: []string{"1:a", "2:b"},
		},
		{
			name: "a blank line keeps its number",
			text: "a\n\nc\n",
			want: []string{"1:a", "2:", "3:c"},
		},
		{
			name: "CRLF is not shown as content",
			text: "a\r\nb\r\n",
			want: []string{"1:a", "2:b"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			var out bytes.Buffer

			g.Expect(render(&out, "a/b.ts", c.text)).To(Succeed())

			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			g.Expect(lines[0]).To(Equal("[a/b.ts#" + anchor.Tag(c.text) + "]"))
			g.Expect(lines[1:]).To(Equal(c.want))
		})
	}
}

// TestApplyDrivesTheApplier is the shell test the iteration asks for: a person can
// take a listing, write a patch against it, and see what the applier does.
func TestApplyDrivesTheApplier(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		patch   string
		out     string
		refused string
	}{
		{
			name:  "a substitution",
			file:  "const n = 1;\n",
			patch: "SUB 1:\n-const\n+let",
			out:   "@@ -1,1 +1,1 @@\n-const n = 1;\n+let n = 1;\n",
		},
		{
			name:    "old text that is not there",
			file:    "const n = 1;\n",
			patch:   "PUT 1.=1:\n-const q = 9;\n+let n = 1;",
			refused: "Line 1 is `const n = 1;`",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "demo.ts")
			g.Expect(os.WriteFile(path, []byte(c.file), 0o600)).To(Succeed())

			reply := "[" + path + "#" + anchor.Tag(c.file) + "]\n" + c.patch + "\n"
			var out bytes.Buffer
			args := []string{"ratchet-dev", "apply", "--file", path}

			err := drive(t, args, strings.NewReader(reply), &out)

			if c.refused != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(out.String()).To(ContainSubstring(c.refused))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out.String()).To(Equal(c.out))
		})
	}
}

// TestApplyWritesNothing is the property the applier's own import gate protects, seen
// from the outside: driving the command leaves the file as it was.
func TestApplyWritesNothing(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.ts")
	before := "const n = 1;\n"
	g.Expect(os.WriteFile(path, []byte(before), 0o600)).To(Succeed())

	reply := "[" + path + "#" + anchor.Tag(before) + "]\nSUB 1:\n-const\n+let\n"
	var out bytes.Buffer

	g.Expect(drive(t, []string{"ratchet-dev", "apply", "--file", path}, strings.NewReader(reply), &out)).To(Succeed())

	after, err := os.ReadFile(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(after)).To(Equal(before), "the applier reports what would happen and changes nothing")
}

// drive runs one subcommand with its streams supplied, so a test can read what a
// person would have seen.
func drive(t *testing.T, args []string, in *strings.Reader, out *bytes.Buffer) error {
	t.Helper()
	cmd := command()
	for _, sub := range cmd.Commands {
		if sub.Name != "apply" {
			continue
		}
		sub.Action = func(ctx context.Context, c *cli.Command) error {
			return runApply(ctx, c, in, out)
		}
	}
	return cmd.Run(t.Context(), args)
}
