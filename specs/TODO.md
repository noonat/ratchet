# TODO

Not a spec. A place to put ideas and observations while working on something
else, so they survive without derailing what is in progress. Items graduate into
a numbered spec, or get deleted once they turn out not to matter.

Nothing here is committed to. There are no checkboxes on purpose: `backlog`
reads a `##` heading containing checkboxes as an iteration, and this file must
not become work it tracks.

## Edit applier

- Similarity matching stays out, but a **near-miss report** might be worth it:
  when a refused edit's old text differs from the file by whitespace alone, say
  so in the refusal instead of leaving the model to guess. Measured evidence
  exists that a diagnosed refusal is worth about 45% of a correct edit, and an
  undiagnosed one much less.

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

  Renaming is not free today. The fixtures header keys on the file name, so a
  renamed journal reads as a new source while its records under the old name are
  kept as an absent one, and the committed file silently doubles. Neither guard
  catches it: the hashes are unchanged and the count grows rather than drops.
  Detecting it is cheap, because the hash is already recorded. A present journal
  whose hash matches an absent recorded source under another name is a rename.

## Open questions carried over

- Does an anchor need to survive a file being renamed? Today it does not, and a
  rename during an iteration would refuse every subsequent edit to that file.
- Nothing yet writes to disk. When something does, decide whether a write is
  atomic per file or per patch, and what happens when the second hunk of a patch
  fails validation after the first was applied.
