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

	"github.com/urfave/cli/v3"

	"ratchet/internal/dev/fixture"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ratchet-dev: %+v\n", err)
		os.Exit(1)
	}
}

// run is everything main does, so that a failure returns rather than exits.
//
// Exiting from wherever a call happened to fail skips every deferred close, and it
// puts the decision about what an error looks like in a dozen places instead of one.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := &cli.Command{
		Name:  "ratchet-dev",
		Usage: "tooling for working on ratchet, run from the repository root",
		Commands: []*cli.Command{
			fixturesCmd(),
		},
	}
	return cmd.Run(ctx, os.Args)
}

// fixturesCmd rebuilds the replay fixtures from the journals.
func fixturesCmd() *cli.Command {
	return &cli.Command{
		Name:  "fixtures",
		Usage: "rebuild testdata/fixtures.jsonl from the journals in journals/",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "accept a rebuild the guards would otherwise refuse",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return fixture.Refresh(c.Bool("force"), os.Stdout)
		},
	}
}
