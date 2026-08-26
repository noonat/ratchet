package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"ratchet/internal/agent"
	"ratchet/internal/executor"
	"ratchet/internal/executor/tool"
)

// driveCmd runs one iteration against a host, which is the only thing here that
// needs a model.
func driveCmd() *cli.Command {
	return &cli.Command{
		Name:  "drive",
		Usage: "run one iteration against a model host and report what happened",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "model",
				Usage:    "the model to run",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "host",
				Usage:    "host or host:port; keep a default in a shell alias or the environment",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "root",
				Usage:    "the directory the session may touch",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "task",
				Usage:    "what to do, as the model is told it",
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:  "file",
				Usage: "a file the iteration names; repeat for several",
			},
			&cli.IntFlag{
				Name:  "num-ctx",
				Value: 20480,
			},
			&cli.IntFlag{
				Name:  "predict",
				Value: 12288,
			},
			&cli.StringFlag{
				Name:  "think",
				Usage: "the think field, empty to leave it off",
			},
			&cli.IntFlag{
				Name:  "max-turns",
				Value: agent.DefaultMaxTurns,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return runDrive(ctx, os.Stdout, driveArgs{
				model:    c.String("model"),
				host:     c.String("host"),
				root:     c.String("root"),
				task:     c.String("task"),
				files:    c.StringSlice("file"),
				numCtx:   c.Int("num-ctx"),
				predict:  c.Int("predict"),
				think:    c.String("think"),
				maxTurns: c.Int("max-turns"),
			})
		},
	}
}

// driveArgs is what one run needs, gathered so the action stays a mapping and the
// work below can be driven from a test.
type driveArgs struct {
	model    string
	host     string
	root     string
	task     string
	files    []string
	numCtx   int
	predict  int
	think    string
	maxTurns int

	// provider replaces the connection to a host. Only a test sets it; the
	// command leaves it nil and gets the host named by --host.
	provider agent.Provider
}

// runDrive runs one iteration and prints the thread and the outcome.
//
// It prints the whole thread because the point of the first run against a host is
// to see what the model actually did, and a summary is what someone writes once
// they already know.
func runDrive(ctx context.Context, out io.Writer, a driveArgs) error {
	if _, err := os.Stat(a.root); err != nil {
		return errors.Wrapf(err, "the session root")
	}
	p := a.provider
	if p == nil {
		p = agent.NewOllama(a.host)
	}
	tools := tool.NewTools(tool.NewSession(a.root, a.files...))
	report, runErr := agent.Run(ctx, p, tools, agent.Iteration{
		Model:       a.model,
		System:      executor.System(),
		Task:        executor.Task(a.files, a.task),
		NumCtx:      a.numCtx,
		NeedContext: a.numCtx,
		Predict:     a.predict,
		Think:       a.think,
		MaxTurns:    a.maxTurns,
	})

	for i, m := range report.Thread {
		fmt.Fprintf(out, "\n--- %d %s %s\n", i, m.Role, m.ToolName)
		if m.Content != "" {
			fmt.Fprintln(out, indent(m.Content))
		}
		for _, call := range m.ToolCalls {
			fmt.Fprintf(out, "  calls %s %v\n", call.Name, call.Args)
		}
	}
	fmt.Fprintf(out, "\nstopped: %s after %d turns\n", report.Stop, report.Turns)
	if report.Said != "" {
		fmt.Fprintf(out, "said: %s\n", report.Said)
	}
	return runErr
}

// indent shifts a message's text so the thread reads as a transcript.
func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}
