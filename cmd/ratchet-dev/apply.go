package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"ratchet/internal/anchor"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

// readCmd renders a file the way a read renders it for a model.
//
// It exists because `apply` is unusable without it. A patch carries the anchor of the
// read it was written against, and a person at a shell has no read to copy one from.
func readCmd() *cli.Command {
	return &cli.Command{
		Name:  "read",
		Usage: "print a file as a tagged, numbered listing, the way a model is shown one",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "file",
				Usage:    "the file to render",
				Required: true,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			path := c.String("file")
			text, err := os.ReadFile(path)
			if err != nil {
				return errors.Wrapf(err, "reading %s", path)
			}
			return render(os.Stdout, path, string(text))
		},
	}
}

// applyCmd drives the applier on one file and one patch.
func applyCmd() *cli.Command {
	return &cli.Command{
		Name:  "apply",
		Usage: "apply a patch to a file and print the result, without writing",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "file",
				Usage:    "the file to edit",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "patch",
				Usage: "the reply to apply; reads standard input when absent",
			},
			&cli.IntFlag{
				Name:  "max-hunks",
				Value: 1,
				Usage: "how many changes were asked for",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return runApply(ctx, c, os.Stdin, os.Stdout)
		},
	}
}

// runApply is the body, taking its streams so it can be driven by something other
// than a terminal.
func runApply(ctx context.Context, c *cli.Command, in io.Reader, out io.Writer) error {
	path := c.String("file")
	file, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "reading %s", path)
	}

	reply, err := patchText(c.String("patch"), in)
	if err != nil {
		return err
	}

	p, err := patch.Parse(reply)
	if err != nil {
		return err
	}

	// This invocation is the read. There is no session to have issued an anchor, so
	// the file being edited is the one being served, and the provenance check passes
	// because it is this command that served it.
	reads := anchor.NewReads()
	reads.Record(p.Path, anchor.NewSnapshot(string(file)))

	res, applyErr := edit.Apply(ctx, reads, *p, string(file), edit.Options{
		MaxHunks: c.Int("max-hunks"),
	})
	if applyErr != nil {
		return report(out, res, applyErr)
	}
	_, err = fmt.Fprint(out, res.Diff)
	return errors.Wrap(err, "printing the diff")
}

// report prints a refusal the way the applier hands it back: what is wrong, the file
// as it stands, and the attempt itself when there was one.
//
// All three, because SWE-agent's ablation is the argument for all three. Without the
// error the model misdiagnoses, without its own attempt it sends the same edit again,
// and without the current file it edits against a memory four turns old.
func report(out io.Writer, res edit.Result, cause error) error {
	if _, err := fmt.Fprintf(out, "refused: %v\n", cause); err != nil {
		return errors.Wrap(err, "printing the refusal")
	}
	if res.Would != "" {
		if _, err := fmt.Fprintf(out, "\nthe edit would have produced:\n%s", res.Would); err != nil {
			return errors.Wrap(err, "printing the attempt")
		}
	}
	return errors.Mark(cause, errReported)
}

// patchText reads the reply from a file, or from the stream when no file was named.
func patchText(path string, in io.Reader) (string, error) {
	if path == "" {
		text, err := io.ReadAll(in)
		return string(text), errors.Wrap(err, "reading the patch from standard input")
	}
	text, err := os.ReadFile(path)
	return string(text), errors.Wrapf(err, "reading %s", path)
}

// render writes the listing a model is shown: the section header carrying the path
// and the file's tag, then every line with its number.
func render(out io.Writer, path, text string) error {
	snap := anchor.NewSnapshot(text)
	if _, err := fmt.Fprintf(out, "[%s#%s]\n", path, snap.Tag); err != nil {
		return errors.Wrap(err, "printing the header")
	}
	for i, line := range anchor.Lines(text) {
		if _, err := fmt.Fprintf(out, "%d:%s\n", i+1, line); err != nil {
			return errors.Wrap(err, "printing a line")
		}
	}
	return nil
}
