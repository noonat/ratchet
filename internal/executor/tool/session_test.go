package tool

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"ratchet/internal/anchor"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

// write puts a file under a fresh root and hands back both.
func write(g *WithT, t *testing.T, name, text string) (string, *Session) {
	g.THelper()
	root := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(root, name), []byte(text), 0o644)).To(Succeed())
	return root, NewSession(root)
}

// put is a one-line whole-line replacement against a file that was read whole.
func put(text, path string, line int, old, new string) patch.Patch {
	return patch.Patch{
		Path: path,
		Tag:  anchor.Tag(text),
		Hunks: []patch.Hunk{{
			Kind: patch.KindPut, Line: line, End: line,
			Old: []string{old}, New: []string{new},
		}},
	}
}

// TestAReadOutlivesTheCallThatServedIt is the reason a session exists.
//
// An anchor arrives with an edit some turns after the read that produced it, and
// two unrelated reads in between are the ordinary case. A Reads built per call can
// only hold the file being edited, which makes the provenance rule unfailable and
// therefore untested: every anchor was served a statement earlier.
func TestAReadOutlivesTheCallThatServedIt(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	root, s := write(g, t, "a.ts", text)
	g.Expect(os.WriteFile(filepath.Join(root, "b.ts"), []byte("x\n"), 0o644)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(root, "c.ts"), []byte("y\n"), 0o644)).To(Succeed())

	_, err := s.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred())
	_, err = s.Read("b.ts")
	g.Expect(err).NotTo(HaveOccurred())
	_, err = s.Read("c.ts")
	g.Expect(err).NotTo(HaveOccurred())

	res, err := s.Edit(t.Context(), put(text, "a.ts", 2, "two", "2"), edit.Options{})

	g.Expect(err).NotTo(HaveOccurred(), "the read three calls ago is the one that counts")
	g.Expect(res.Edit.Text).To(Equal("one\n2\nthree\n"))
}

// TestAnEditAgainstAnUnreadPathIsRefused is the other direction, and the one that
// cannot fail today because nothing records.
func TestAnEditAgainstAnUnreadPathIsRefused(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\n"
	_, s := write(g, t, "a.ts", text)

	_, err := s.Edit(t.Context(), put(text, "a.ts", 1, "one", "1"), edit.Options{})

	g.Expect(err).To(HaveOccurred(), "no read served this path in this session")
}

// TestAResolvedEditReachesTheFile pairs the two halves. The applier decides and this
// package writes, so the check is that the bytes on disk match what was decided.
func TestAResolvedEditReachesTheFile(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	root, s := write(g, t, "a.ts", text)

	_, err := s.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred())
	res, err := s.Edit(t.Context(), put(text, "a.ts", 2, "two", "2"), edit.Options{})
	g.Expect(err).NotTo(HaveOccurred())

	on, err := os.ReadFile(filepath.Join(root, "a.ts"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(on)).To(Equal("one\n2\nthree\n"))
	g.Expect(res.Edit.Text).To(Equal(string(on)), "what was decided is what was written")
}

// TestTheReturnedTagCarriesTheNextEdit closes the loop. The file changed, so the
// anchor the caller holds is stale, and a second edit with no read between would be
// refused for something it cannot act on. Handing back a live tag is the measured
// flow, and provenance rather than validation is what keeps that safe.
func TestTheReturnedTagCarriesTheNextEdit(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\n"
	root, s := write(g, t, "a.ts", text)

	_, err := s.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred())
	first, err := s.Edit(t.Context(), put(text, "a.ts", 1, "one", "1"), edit.Options{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.Tag).NotTo(BeEmpty())

	second := put("", "a.ts", 2, "two", "2")
	second.Tag = first.Tag
	_, err = s.Edit(t.Context(), second, edit.Options{})

	g.Expect(err).NotTo(HaveOccurred(), "the tag an edit returns is one the next edit may carry")
	on, err := os.ReadFile(filepath.Join(root, "a.ts"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(on)).To(Equal("1\n2\n"))
}

// TestARefusedEditTouchesNothing is the property the whole separation exists for.
// The applier cannot reach a file, so a refusal has nothing to undo; this asserts
// the same of the layer that can.
func TestARefusedEditTouchesNothing(t *testing.T) {
	const text = "one\ntwo\nthree\n"

	cases := []struct {
		name  string
		patch func() patch.Patch
		read  bool
	}{
		{
			name:  "an anchor no read in this session served",
			patch: func() patch.Patch { return put(text, "a.ts", 2, "two", "2") },
			read:  false,
		},
		{
			name: "a tag that does not match the file",
			patch: func() patch.Patch {
				p := put(text, "a.ts", 2, "two", "2")
				p.Tag = "0000"
				return p
			},
			read: true,
		},
		{
			name:  "an old row that is not what the file says",
			patch: func() patch.Patch { return put(text, "a.ts", 2, "TWO", "2") },
			read:  true,
		},
		{
			name:  "a line past the end of the file",
			patch: func() patch.Patch { return put(text, "a.ts", 9, "nine", "9") },
			read:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			root, s := write(g, t, "a.ts", text)
			if c.read {
				_, err := s.Read("a.ts")
				g.Expect(err).NotTo(HaveOccurred())
			}

			_, err := s.Edit(t.Context(), c.patch(), edit.Options{})

			g.Expect(err).To(HaveOccurred())
			on, readErr := os.ReadFile(filepath.Join(root, "a.ts"))
			g.Expect(readErr).NotTo(HaveOccurred())
			g.Expect(string(on)).To(Equal(text), "a refused edit leaves the file as it was")
		})
	}
}

// TestAnEditedFileKeepsItsMode guards the rename. CreateTemp makes 0600 and a
// rename carries that over, so without the explicit chmod every edit would narrow
// the file it wrote.
func TestAnEditedFileKeepsItsMode(t *testing.T) {
	const text = "one\ntwo\n"

	modes := []os.FileMode{0o600, 0o644, 0o755}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			g := NewWithT(t)
			root, s := write(g, t, "a.ts", text)
			target := filepath.Join(root, "a.ts")
			g.Expect(os.Chmod(target, mode)).To(Succeed())

			_, err := s.Read("a.ts")
			g.Expect(err).NotTo(HaveOccurred())
			_, err = s.Edit(t.Context(), put(text, "a.ts", 1, "one", "1"), edit.Options{})
			g.Expect(err).NotTo(HaveOccurred())

			info, err := os.Stat(target)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(info.Mode().Perm()).To(Equal(mode))
		})
	}
}

// TestAPathOutsideTheRootIsRefused. The root is the only statement anyone has made
// about what this session may touch, and a mistyped address should not widen it.
func TestAPathOutsideTheRootIsRefused(t *testing.T) {
	g := NewWithT(t)
	_, s := write(g, t, "a.ts", "one\n")

	_, err := s.Read("../outside.ts")

	g.Expect(err).To(HaveOccurred())
}

// TestPreviewDecidesWithoutWriting is the other half of the pair. A person at a
// shell wants to know what an edit would do, and the applier already computes it;
// the only difference is whether the result reaches the file.
func TestPreviewDecidesWithoutWriting(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\n"
	root, s := write(g, t, "a.ts", text)

	_, err := s.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred())
	res, err := s.Preview(t.Context(), put(text, "a.ts", 1, "one", "1"), edit.Options{})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res.Edit.Text).To(Equal("1\ntwo\n"), "the decision is the same one Edit makes")
	g.Expect(res.Tag).To(BeEmpty(), "nothing was written, so there is no new tag to carry")
	on, err := os.ReadFile(filepath.Join(root, "a.ts"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(on)).To(Equal(text), "the file is untouched")
}

// TestALinkOutOfTheRootIsRefused closes the hole a lexical check leaves. A link
// inside the root can point anywhere, so the root stops meaning anything unless the
// target is checked too — and a write through one would replace the link with a
// regular file, leaving the model certain it edited what it read.
func TestALinkOutOfTheRootIsRefused(t *testing.T) {
	g := NewWithT(t)
	root, s := write(g, t, "a.ts", "one\n")
	outside := filepath.Join(filepath.Dir(root), "outside.ts")
	g.Expect(os.WriteFile(outside, []byte("classified\n"), 0o644)).To(Succeed())
	g.Expect(os.Symlink(outside, filepath.Join(root, "link.ts"))).To(Succeed())

	_, err := s.Read("link.ts")

	g.Expect(err).To(HaveOccurred(), "the link's target is what a read would serve")
}

// TestAnEditThroughALinkKeepsTheLink is the other half. A link inside the root is
// legitimate, and renaming a temporary file over it would turn it into a regular
// file while its target kept the old content.
func TestAnEditThroughALinkKeepsTheLink(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\n"
	root, s := write(g, t, "real.ts", text)
	link := filepath.Join(root, "link.ts")
	g.Expect(os.Symlink("real.ts", link)).To(Succeed())

	_, err := s.Read("link.ts")
	g.Expect(err).NotTo(HaveOccurred())
	_, err = s.Edit(t.Context(), put(text, "link.ts", 1, "one", "1"), edit.Options{})
	g.Expect(err).NotTo(HaveOccurred())

	info, err := os.Lstat(link)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(info.Mode()&os.ModeSymlink).NotTo(BeZero(), "the link is still a link")
	through, err := os.ReadFile(filepath.Join(root, "real.ts"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(through)).To(Equal("1\ntwo\n"), "the edit reached the file the read served")
}

// TestReadServesWholeFiles pins the assumption the post-write snapshot rests on.
// Recording a whole-file snapshot after an edit is sound while every read is whole;
// the moment a windowed read exists, one edit would mark lines nobody saw as shown.
func TestReadServesWholeFiles(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	_, s := write(g, t, "a.ts", text)

	listing, err := s.Read("a.ts")

	g.Expect(err).NotTo(HaveOccurred())
	displayed := []string{"1:one", "2:two", "3:three"}

	for i, line := range displayed {
		g.Expect(listing).To(ContainSubstring(line), "line %d is displayed", i+1)
	}
}

// TestAFileTheIterationDidNotNameIsRefused is the promise the prompt made and
// nothing kept.
//
// `Task` tells the model which files it may touch. Until the list reached the
// session, a model editing a different file under the root was told the edit
// applied, so an iteration could write outside its declared scope and report
// success. The wording is the architecture's: the path is not in this
// iteration's files.
func TestAFileTheIterationDidNotNameIsRefused(t *testing.T) {
	g := NewWithT(t)
	const text = "one\ntwo\nthree\n"
	root, _ := write(g, t, "a.ts", text)
	g.Expect(os.WriteFile(filepath.Join(root, "b.ts"), []byte(text), 0o644)).To(Succeed())

	scoped := NewSession(root, "a.ts")

	_, err := scoped.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred(), "the file the iteration named is served")

	_, err = scoped.Read("b.ts")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not in this iteration's files"))
	g.Expect(err.Error()).To(ContainSubstring("a.ts"), "the refusal names what is allowed")

	// An edit is refused on the same grounds, before the applier runs, so the
	// answer does not depend on whether the patch would have resolved.
	_, err = scoped.Edit(t.Context(), put(text, "b.ts", 2, "two", "2"), edit.Options{})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not in this iteration's files"))

	after, readErr := os.ReadFile(filepath.Join(root, "b.ts"))
	g.Expect(readErr).NotTo(HaveOccurred())
	g.Expect(string(after)).To(Equal(text), "and nothing was written")
}

// TestAnEmptySessionServesTheWholeRoot keeps every existing caller working. A
// session given no list is the behaviour that shipped before this.
func TestAnEmptySessionServesTheWholeRoot(t *testing.T) {
	g := NewWithT(t)
	root, open := write(g, t, "a.ts", "one\n")
	g.Expect(os.WriteFile(filepath.Join(root, "b.ts"), []byte("x\n"), 0o644)).To(Succeed())

	_, err := open.Read("a.ts")
	g.Expect(err).NotTo(HaveOccurred())
	_, err = open.Read("b.ts")
	g.Expect(err).NotTo(HaveOccurred())
}
