package convention

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// private matches the things a public repository must not name: addresses on a
// home network, addresses on a private tailnet, and the two machines this was
// developed against.
//
// The documentation ranges are deliberately absent, because they are the
// replacement: RFC 5737 gives 192.0.2.0/24, 198.51.100.0/24 and 203.0.113.0/24
// for IPv4, and RFC 3849 gives 2001:db8::/32 for IPv6. Nothing there resolves,
// so an example cannot be mistaken for a live host.
// pattern is one thing a public repository must not name. A declared type rather
// than the anonymous struct a table uses, because this is configuration read by
// one check and not a list of cases each needing its own subtest.
type pattern struct {
	// name says what was found, for the failure message.
	name string
	// re matches it.
	re *regexp.Regexp
}

var private = []pattern{
	{"an RFC 1918 address", regexp.MustCompile(`\b(?:10|192\.168)\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)},
	{"an RFC 1918 address", regexp.MustCompile(`\b172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}\b`)},
	{"a tailnet address", regexp.MustCompile(`\b100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.\d{1,3}\.\d{1,3}\b`)},
	{"a tailnet address", regexp.MustCompile(`(?i)\bfd7a:[0-9a-f]{1,4}:`)},
	{"a machine on a private network", regexp.MustCompile(`(?i)\bthor\b`)},
	{"a machine on a private network", regexp.MustCompile(`@block\b`)},
}

// TestNothingNamesAPrivateNetwork keeps one person's network out of a repository
// that will be public.
//
// It is a whole-tree text scan rather than a Go-file walk, because the leak this
// was written for was six lines of prose in two design documents and one flag
// default. Four of the eight were real network identifiers.
func TestNothingNamesAPrivateNetwork(t *testing.T) {
	g := NewWithT(t)
	root, err := filepath.Abs("../..")
	g.Expect(err).NotTo(HaveOccurred())

	var found []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// This package is skipped because the rules have to name what they
			// forbid. Nothing else is skipped: a leak hides in whatever file
			// nobody thought to check.
			if d.Name() == ".git" || d.Name() == "node_modules" ||
				path == filepath.Join(root, "internal", "convention") {
				return filepath.SkipDir
			}
			return nil
		}
		if !text(path) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(body), "\n") {
			for _, p := range private {
				if p.re.MatchString(line) {
					rel, _ := filepath.Rel(root, path)
					leak := fmt.Sprintf(
						"%s:%d names %s: %s",
						rel,
						n+1,
						p.name,
						strings.TrimSpace(line),
					)
					found = append(found, leak)
				}
			}
		}
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(
		BeEmpty(),
		"this repository will be public; use the documentation ranges instead",
	)
}

// text reports whether a path is worth reading as source or prose.
func text(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".md", ".json", ".yaml", ".yml", ".ts", ".js", ".py", ".sh", ".txt":
		return true
	}
	return filepath.Base(path) == "Makefile"
}
