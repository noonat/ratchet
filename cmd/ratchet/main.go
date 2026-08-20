// Command ratchet is the single binary a user installs. It currently does nothing:
// iteration 1 exists to put a module, a Makefile and a working gate on disk, so that
// closing it is the first proof the gate can run at all.
//
// Tooling for working on this repository is `ratchet-dev`, kept separate because a
// command tree a user sees is a promise and none of that tooling works outside a
// checkout.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ratchet: %+v\n", err)
		os.Exit(1)
	}
}

// run is everything main does, so a failure returns rather than exits. Exiting from
// wherever a call failed skips every deferred close and scatters the decision about
// what an error looks like.
func run() error {
	_, err := fmt.Println("ratchet")
	return err
}
