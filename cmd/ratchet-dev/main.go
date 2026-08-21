// Command ratchet-dev is the tooling for working on this repo, kept out of the
// `ratchet` binary because none of it is part of the product.
//
// `ratchet` is what a user installs. Its command tree is a promise, and a subcommand
// that only works inside a checkout of this repository does not belong in it: the
// fixtures live here, the journals are gitignored, and neither exists on the machine
// of anyone who installed the binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"ratchet/internal/dev/fixture"
	"ratchet/internal/dev/replay"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

func main() {
	err := run()
	switch {
	case err == nil:
	case errors.Is(err, errReported):
		// Its message is already on the command's own output. Printing it again, and
		// with a stack, buries what a person came here to read.
		os.Exit(1)
	case decided(err):
		fmt.Fprintf(os.Stderr, "ratchet-dev: %v\n", err)
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "ratchet-dev: %+v\n", err)
		os.Exit(1)
	}
}

// errReported marks an error whose message has already been written where the person
// running the command will see it.
var errReported = errors.New("already reported")

// decided reports whether an error is a decision this tool made rather than a fault
// it ran into.
//
// A decision is a sentence written for whoever ran the command, and a stack under it
// is noise. A fault is the opposite: the stack is the part that says which of several
// file operations produced it.
//
// As rather than Is, because Is tests identity against a sentinel value and these
// three carry the message they were built with, so there is no one value to compare
// against. A single check would need an interface all three implement, and the two
// packages most likely to go public are written to depend on nothing else in the
// module; a marker interface would cost that for no gain here.
func decided(err error) bool {
	var refusal *edit.Refusal
	var fault *patch.Fault
	var refused *fixture.Refusal
	return errors.As(err, &refusal) || errors.As(err, &fault) || errors.As(err, &refused)
}

// run is everything main does, so that a failure returns rather than exits.
//
// Exiting from wherever a call happened to fail skips every deferred close, and it
// puts the decision about what an error looks like in a dozen places instead of one.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return command().Run(ctx, os.Args)
}

// command is the tree, built by a function so a test can drive one subcommand with
// its streams supplied.
func command() *cli.Command {
	return &cli.Command{
		Name:  "ratchet-dev",
		Usage: "tooling for working on ratchet, run from the repository root",
		Commands: []*cli.Command{
			applyCmd(),
			fixturesCmd(),
			readCmd(),
			replayCmd(),
		},
	}
}

// fixturesCmd rebuilds the replay fixtures from the journals.
func fixturesCmd() *cli.Command {
	return &cli.Command{
		Name:  "fixtures",
		Usage: "rebuild testdata/fixtures.jsonl from the journals in journals/",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "accept journals that changed since the fixtures were built; duplicates are still refused",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return fixture.Refresh(c.Bool("force"), os.Stdout)
		},
	}
}

// replayCmd reports how often the applier agrees with the harness.
func replayCmd() *cli.Command {
	return &cli.Command{
		Name:  "replay",
		Usage: "report how often the applier reaches the harness's verdict, per patch form",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "disagreements",
				Usage: "print every disagreement, which is what adjudicating one needs",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return replay.Summarise(os.Stdout, c.Bool("disagreements"))
		},
	}
}
