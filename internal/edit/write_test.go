package edit

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// TestNothingHereCanReachAFile enforces that this package imports only what it needs,
// none of which can open a file.
//
// The spec asks for a test asserting the file on disk did not change after a refused
// edit. Written directly that test can never fail: Apply takes the file's text and
// returns text, so there is no path for it to write to. The property that makes it
// true is the one worth gating, and the gate belongs at the import rather than at the
// call, because a package that can open a file will eventually be given a reason to.
//
// An allowlist rather than a list of banned packages. A banlist of five stdlib paths
// let through `io/fs`, `net/http`, `archive/zip`, and any package in this repo that
// writes. Naming what is permitted cannot be incomplete in that direction, and adding
// an entry is then a deliberate act.
//
// Test files are exempt, which is why this one may read a directory. A fixture on disk
// is not a write by the applier.
func TestNothingHereCanReachAFile(t *testing.T) {
	allowed := map[string]struct{}{
		`"context"`:                       {},
		`"fmt"`:                           {},
		`"strings"`:                       {},
		`"unicode"`:                       {},
		`"github.com/cockroachdb/errors"`: {},
		`"ratchet/internal/anchor"`:       {},
		`"ratchet/internal/patch"`:        {},
	}

	g := NewWithT(t)
	entries, err := os.ReadDir(".")
	g.Expect(err).NotTo(HaveOccurred())

	examined := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		examined++
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
			g.Expect(err).NotTo(HaveOccurred())

			var found []string
			for _, imp := range file.Imports {
				if _, ok := allowed[imp.Path.Value]; !ok {
					found = append(found, imp.Path.Value)
				}
			}
			g.Expect(found).
				To(BeEmpty(), "this list is the whole of what the applier may reach; writing belongs to whatever calls it")
		})
	}

	// Nothing examined is a green run that checked nothing, which is what a package
	// emptied or renamed would produce.
	g.Expect(examined).NotTo(BeZero(), "found no source files to check")
}
