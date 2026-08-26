package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/agent"
)

// asking returns a reply that calls one tool with one argument.
func asking(name, key, value string) agent.Reply {
	return agent.Reply{
		ToolCalls: []agent.ToolCall{
			{
				Name: name,
				Args: map[string]any{key: value},
			},
		},
	}
}

// TestDriveWritesEveryTurnToTheTranscript is what the command is for: someone
// watching a first run against a host needs to see what the model sent, what
// came back, and where it stopped.
//
// It drives the real tools against a real file and only the host is scripted,
// because a transcript that is right about a stub and wrong about a session is
// worth nothing.
func TestDriveWritesEveryTurnToTheTranscript(t *testing.T) {
	g := NewWithT(t)

	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "game.js"), []byte("const midpoint = 4\n"), 0o644)
	g.Expect(err).NotTo(HaveOccurred())

	p := &agent.Script{
		Context: 20480,
		Replies: []agent.Reply{
			asking("read", "path", "game.js"),
			asking("edit", "patch", "not a patch"),
			asking("done", "summary", "renamed it"),
		},
	}

	var out bytes.Buffer
	err = runDrive(t.Context(), &out, driveArgs{
		model:    "m",
		root:     root,
		task:     "rename the field",
		files:    []string{"game.js"},
		numCtx:   20480,
		maxTurns: 8,
		provider: p,
	})
	g.Expect(err).NotTo(HaveOccurred())

	got := out.String()
	g.Expect(p.Turns()).To(Equal(3))

	// The call the model asked for and the result it was handed are separate
	// turns, so a reader can see which of the two went wrong.
	g.Expect(got).To(ContainSubstring("calls read map[path:game.js]"))
	g.Expect(got).To(ContainSubstring("const midpoint = 4"))
	g.Expect(got).To(ContainSubstring("calls edit map[patch:not a patch]"))
	g.Expect(got).To(ContainSubstring("calls done map[summary:renamed it]"))
	g.Expect(got).To(ContainSubstring("stopped: done after 3 turns"))
	g.Expect(got).To(ContainSubstring("said: renamed it"))
}
