// Command distill rebuilds testdata/fixtures.jsonl from the journals in journals/.
//
// It is what `make fixtures` runs. Kept out of `make check`, because it reads a
// gitignored directory and a gate has to pass on a fresh clone.
//
// Stdlib only, and deliberately not the CLI framework the architecture names for
// `ratchet` itself. That framework is there for one measured reason, flags that
// parse in any position, because argument order is what a model gets wrong. This
// command takes no arguments and no model runs it: `make` does. Adding a
// dependency whose one purpose is unused is not consistency, and the third-party
// list is meant to be short enough that every entry has a reason.
//
// FORCE is an environment variable rather than a flag because `make` passes it
// that way: `FORCE=1 make fixtures`.
package main

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"

	"ratchet/internal/fixture"
)

func main() {
	force := os.Getenv("FORCE") == "1"
	set, err := fixture.Rebuild("journals", "testdata/fixtures.jsonl", force)
	if err != nil {
		fail(err)
	}
	if err := fixture.Save("testdata/fixtures.jsonl", set); err != nil {
		fail(err)
	}
	for _, s := range set.Sources {
		// %.12s rather than a slice: a header that was truncated or edited by hand
		// would make a slice panic where this prints what is there.
		fmt.Printf("  %-28s %6d records  %.12s\n", s.Journal, s.Records, s.SHA256)
	}
	fmt.Printf("  %d records from %d journals\n", len(set.Records), len(set.Sources))
}

// fail reports and exits. A refusal is a message written for whoever ran the
// target, so it prints as one; anything else is a fault, and its stack says which
// of several file operations produced it.
func fail(err error) {
	var refusal *fixture.Refusal
	if errors.As(err, &refusal) {
		fmt.Fprintf(os.Stderr, "distill: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "distill: %+v\n", err)
	}
	os.Exit(1)
}
