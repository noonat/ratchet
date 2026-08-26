// Package executor holds the executor seat: what it is told, and what it is
// allowed to spend.
//
// The prompt lives here rather than with the tools because the tools are the same
// calls whoever is driving them, and what to say about them is the seat's.
package executor

import (
	"fmt"
	"strings"

	"ratchet/internal/executor/tool"
)

// System is the standing instruction for one iteration.
//
// Short on purpose. Every rule in it is one the tools enforce anyway, so the
// prompt teaches the notation and the two ways to stop, and lets a refusal teach
// the rest. A refusal is measured text that recovers 221 of 494 diagnosed
// failures; a paragraph of prose warning about the same mistake is not.
func System() string {
	return strings.TrimSpace(fmt.Sprintf(`
You are changing files in a repository. Work in small steps and stop when done.

Read a file before editing it. A read returns a header naming the file and a
four-character tag, then numbered lines:

    [path/to/file.go#3449]
    1:package main
    2:
    3:func main() {

An edit is a patch in the same notation: the header, copied from the read you are
editing against, then one change. Two forms:

    [path/to/file.go#3449]
    PUT 3.=3:
    -func main() {
    +func run() error {

    [path/to/file.go#3449]
    SUB 3:
    -main
    +run

PUT replaces whole lines and needs one %s row for every line in its range. SUB
replaces one fragment inside one line. The %s rows state what is there now and
are checked before anything is written, so copy them exactly.

The tag must come from a read you were served. If an edit is refused, the refusal
says what is wrong; fix that and try again.

Stop with %s when the work is finished, or %s when it cannot be, and say why.
`, "`-`", "`-`", tool.NameDone, tool.NameBlocked))
}

// Task renders the work one iteration is given.
func Task(files []string, instruction string) string {
	var b strings.Builder
	b.WriteString(instruction)
	if len(files) > 0 {
		fmt.Fprintf(&b, "\n\nThe files you may touch: %s", strings.Join(files, ", "))
	}
	return b.String()
}
