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

## Open questions carried over

- Does an anchor need to survive a file being renamed? Today it does not, and a
  rename during an iteration would refuse every subsequent edit to that file.
- Nothing yet writes to disk. When something does, decide whether a write is
  atomic per file or per patch, and what happens when the second hunk of a patch
  fails validation after the first was applied.
