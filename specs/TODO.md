# TODO

Not a spec. A place to put ideas and observations while working on something
else, so they survive without derailing what is in progress. Items graduate into
a numbered spec, or get deleted once they turn out not to matter.

Nothing here is committed to. There are no checkboxes on purpose: `backlog`
reads a `##` heading containing checkboxes as an iteration, and this file must
not become work it tracks.

`Next` is the one ordered section. Everything below it is an unordered parking
lot, which is what the rest of this file is for.

## Next

Ordered by what a mistake would cost to discover late, not by what has the best
evidence behind it. Early on those are opposites, because evidence collects
around what is already built, so ranking by evidence ranks the least risky work
first. Specs already written are worked lowest number first, per the index.

1. **The agent loop, against a model host.** The first end-to-end path, and the
   thing every downstream design decision is currently guessing about. The
   refusal economics the whole design rests on have been measured in a research
   harness and never in a loop this repo owns.

   The session under it holds reads across turns and writes whole files, so the
   provenance rule can now fail in a real path. What has never run is a read
   that is three turns old.

2. **`ratchet qualify`.** Moves journal production into the repo, which
   dissolves the journal-naming problem below rather than guarding it.

3. **Failure injection**, per the list below. Needs a loop to inject into, so it
   follows 1.

## Testing

- Failure injection, per the architecture: `DropToolCall`, `TruncateAt`,
  `EmptyGeneration`, `LieAboutFinish`, `NetworkFlap`, `ContextShrink`. Every
  field is something a real serving stack has done, and none is implemented yet.
- Replay cassettes need a home and a naming convention before there are more
  than a handful.

## Replay fixtures

- **The journals come from outside this repo, and their names show it.**
  `ratchet qualify <model>` is already designed to run a model against these
  interfaces and score it per form, which is the same measurement a journal
  holds. Once it exists it should write them, and `journals/README.md` stops
  asking a person to copy files out of a harness that will not be here.

- **Rename the journals when that happens.** `edit-candidates`,
  `edit-conclusive` and `edit-sub-v2` say where a run sat in a sequence of
  attempts rather than what it measured, and `conclusive` was a hope rather than
  a description. `edit-gptoss-4k` and `edit-think-low-4k` name a condition and
  have held up. A name that lasts says what varied: the models, the forms, the
  fixtures, the output cap.

  The rename is now a deliberate act rather than a trap. The fixtures header
  keys on the file name, so a renamed journal used to read as a new source while
  its records under the old name were kept as an absent one, and the committed
  file silently doubled: the hashes were unchanged and the count grew rather
  than dropped, so neither existing guard saw it. The distillation refuses a
  twin now and says whether to rename one or remove one, which is what makes the
  renaming above safe to do.

## Open questions carried over

- Does an anchor need to survive a file being renamed? Today it does not, and a
  rename during an iteration would refuse every subsequent edit to that file.
- **Atomic per file or per patch is the same question today, and will not stay
  that way.** A patch names one path, and every check that can refuse runs
  before any text exists, so a refusal cannot leave a file half-edited and a
  second hunk failing validation reverts the first for free. A patch spanning
  two files reopens it: the second file's refusal would have to undo a write
  already on disk, and the applier has no undo.
- **A windowed read has to decide what a write leaves visible.** `Read` serves
  whole files, which is the only reason a snapshot of the edited text is sound.
  A windowed read recording one would mark every line as displayed and retire
  the shown-lines check, so an edit to a line nobody saw would resolve.

## Prose

- **`docs/edit-applier.md:44` spells British** — "the arithmetic favours
  refusing" — on the line spec **002** now cites for the 45%. Worth folding into
  whatever next touches that file.

## What got pulled up and where

- **The near-miss report.** Spec **002**
- **The renamed-journal guard.** Spec **003**

These stay here so a reader who does not know to ask for a spec number can find
out what was done, by whom, and on what evidence.
