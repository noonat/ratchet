package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"ratchet/internal/anchor"
	"ratchet/internal/edit"
	"ratchet/internal/executor/tool"
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
		Usage: "apply a patch to a file and print the diff; --write saves it",
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
			&cli.BoolFlag{
				Name:  "write",
				Usage: "write the result to the file instead of only printing the diff",
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
	reply, err := patchText(c.String("patch"), in)
	if err != nil {
		return err
	}

	p, err := patch.Parse(reply)
	if err != nil {
		return err
	}

	// One invocation is one session, so this reads the file it is about to edit and
	// the provenance check passes because this session served it. The rule bites
	// across turns, and a shell is not turns.
	//
	// The address is taken from --file rather than from the reply's header, because
	// here the header holds whatever path the reader was given, absolute included,
	// and a session resolves an address under its root. --file is what the person
	// meant; the header says which file the model thought it was editing.
	s := tool.NewSession(filepath.Dir(path))
	p.Path = filepath.Base(path)
	if _, err := s.Read(p.Path); err != nil {
		return err
	}

	apply := s.Preview
	if c.Bool("write") {
		apply = s.Edit
	}
	res, applyErr := apply(ctx, *p, edit.Options{MaxHunks: c.Int("max-hunks")})
	if applyErr != nil {
		return report(out, applyErr)
	}
	_, err = fmt.Fprint(out, res.Edit.Diff)
	return errors.Wrap(err, "printing the diff")
}

// report prints a refusal the way the applier hands it back: what is wrong, the
// attempt itself when there was one, and the file as it stands.
//
// All three, because SWE-agent's ablation is the argument for all three. Without the
// error the model misdiagnoses, without its own attempt it sends the same edit again,
// and without the current file it edits against a memory four turns old.
//
// Only a refusal is marked as reported. An edit that fails to write is a fault, and
// suppressing its stack would lose which file operation failed.
func report(out io.Writer, cause error) error {
	var refusal *edit.Refusal
	if !errors.As(cause, &refusal) {
		return cause
	}
	if _, err := fmt.Fprintf(out, "refused: %v\n", cause); err != nil {
		return errors.Wrap(err, "printing the refusal")
	}
	if refusal.RefusedText != "" {
		if _, err := fmt.Fprintf(out, "\nthe edit would have produced:\n%s", refusal.RefusedText); err != nil {
			return errors.Wrap(err, "printing the attempt")
		}
	}
	if refusal.Text != "" {
		if _, err := fmt.Fprintf(out, "\nthe file as it stands:\n%s", refusal.Text); err != nil {
			return errors.Wrap(err, "printing the file")
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
