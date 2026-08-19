---
status: active
created: 2026-08-19T15:00:00Z
updated: 2026-08-19T16:44:24.179226194Z
required_commands:
  - cmd: make check
---

# Ratchet 001: the edit applier

Build the part that takes a patch and a file and either applies the edit or
refuses it. Nothing else. No agent loop, no container, no server.

Build it first for two reasons. Its design is already settled by measurement:
The research notes that preceded this repo say which patch form to implement,
that checking the old line costs nothing, and that two automatic repairs raise
correct body rows from 64% to 99%. [The architecture](../docs/architecture.md)
says how anchors work, how a refusal must read, and what order the stages run
in.

It is also the only part that can be tested against real model output straight
away. The measurement harness recorded 4,894 attempts, each with the raw model
reply and a scored verdict. Replay those replies through the applier and require
it to reach the same verdict. That converts a day of measurement into a test
suite, and every disagreement is a bug in the applier or in the scorer. Both are
worth finding.

## Terms

- **Read**: showing the model a file. Ratchet stamps that view with a tag and
  records which line numbers it displayed.
- **Tag**: four characters identifying the exact content a read served.
- **Anchor**: a tag plus a line number. This is how a patch says which line to
  change.
- **Hunk**: one change within a patch: a line number, the old text, and the new
  text.
- **Patch**: a path, a tag, and one or more hunks.

## Out of scope

The architecture describes five stages: resolve, apply, normalise, validate,
diff-filter. This spec builds the first three. The rest are named here so nobody
goes looking for them.

- No validate stage. Running the language's own checker on the result, and
  attaching a language server, comes later.
- No diff-filter. Subtracting errors that were already there needs validate
  first.
- No writing to disk. `Apply` returns the new text and the caller decides what
  to do with it.
- No agent loop, no container, no server, and no tool call. The applier is a Go
  function. How a model's reply reaches it is a separate question, measured in
  the research notes and not implemented here.

## Decisions

Each of these is settled. The reason is recorded so a later reader can tell a
decision from a guess.

### Use one tag per file, not one per line

`Tag(text)` is the low 16 bits of `xxhash32(normalize(text))`, printed as four
uppercase hex digits, where `normalize` strips trailing whitespace from each
line. A file copied with CRLF line endings, or with trailing spaces trimmed by
an editor, therefore gets the same tag. The per-line alternative was measured.
It cost the weaker two of four models 15 and 19 correct answers out of 30, where
the file tag cost nothing (83 of 90 with it, 83 of 90 without). Its one
advantage is catching a slipped line number when the model copies the right
line's hash. That fired once in 120 attempts, against 41 refusals a file tag
never causes.

### Name the correct anchor in one branch only

If the file is byte-for-byte what the read served, the model mistyped the tag,
so telling it the right one is safe. If the file changed, do not name the
anchor. Naming it would point the model at content it has never seen, which is
the silent wrong-line edit sneaking back in through the error message. The
anchor alone cannot tell the two cases apart, so the tool decides, never the
model.

### Check where an anchor came from, not just whether it matches

Refuse an edit unless a read in this session issued its anchor, even when the
anchor matches the file exactly. Also refuse any line that read did not display.
A matching anchor does not prove the model saw the line. Measured, local models
lifting an identifier out of a rejection message and retrying without reading
again, on roughly one refusal in fifteen. Every later check passes, because the
identifier is correct about a file the model has not read.

### Never match approximately

aider ships an edit-distance matcher at a threshold of 0.8 and disabled it with
a bare `return`. An edit applied to code that merely resembles the target
corrupts the file in a way review does not catch.

### Turn the re-indent repair off per language

Taking indentation from the line being replaced is what raises body rows from
64% to 99%, and it is correct only where indentation carries no meaning. In
Python it cannot express moving a line out of a block, so switch it off for
languages where whitespace matters. Do not apply it everywhere and hope.

### Cut the replay corpus down and commit it

Do not read it from the measurement harness's output directory. That path breaks
as soon as this repo moves, and the journals hold 70MB of prompt text the
applier never looks at. Iteration 4 copies the journals it needs into a
gitignored `journals/`, extracts the few fields it needs, records each source
journal's hash, and commits the result. The test then needs nothing outside this
repo.

Gitignoring the journals has a consequence worth stating here rather than
discovering later: a fresh clone has the corpus and not its sources, so
rebuilding must merge rather than overwrite. See the gate section.

## The gate

Every iteration closes on `make check`, which builds, lints and tests. Linting
runs `gofmt`, `go vet` and `staticcheck`.

Use one command rather than four in the front matter. The same target has to
work when a person types it, and a gate that drifts from what a person runs
stops being believed.

Copy the `gofmt` step from the measurement harness's Makefile instead of writing
a new one. `gofmt -l` exits 0 whether or not it found unformatted files, so the
check has to test the output. The short version,
`gofmt -l . | tee /dev/stderr | (! read)`, behaved differently under two shells.
The Makefile version captures the output and tests it, which is plainer and
already works here.

`make corpus` is a second target and deliberately not part of `make check`. It
rebuilds `testdata/corpus.jsonl` from journals in `journals/`, which is
gitignored, so a fresh clone has the corpus and not the journals it came from. A
gate has to pass on a fresh clone, so a target needing inputs the clone does not
have can never be a gate.

That gitignore forces the target to be additive rather than overwriting. On a
new machine `journals/` is empty, and a target that rebuilt from whatever it
found would replace a committed corpus with nothing. So `make corpus` merges:
records whose source journal is present are regenerated, records whose source
journal is absent are kept. Running it twice over the same journals produces the
same bytes.

Iteration 1 writes the Makefile, so closing it is the first proof the gate runs.

## Iteration 1: the module and the anchor package

- [x] Create `go.mod` for module `ratchet` on Go 1.26, and a `Makefile` with the
      targets `help build clean fmt lint test check` and `help` as the default,
      matching the measurement harness. Iteration 4 adds `corpus`, which is not
      part of `check`
- [x] Write `internal/anchor/anchor.go` with `Normalize(text string) string`,
      which strips trailing whitespace per line and converts CRLF to LF, and
      `Tag(text string) string`, which returns four uppercase hex digits
- [x] Pin `Tag` in `internal/anchor/anchor_test.go` against the vectors already
      checked against the reference library in the measurement harness's own
      tests, and assert that a CRLF copy and a trailing-whitespace copy of the
      same file produce the same tag
- [x] Add `Snapshot{Tag string; Text string; Lines map[int]bool}` and
      `Mint(text string, lines []int) Snapshot`, and test that a partial read
      records only the lines it displayed

> **Completed** 2026-08-19 16:47 UTC
>
> - `make check` — 265ms

## Iteration 2: the patch language, parsed

- [x] Write `internal/patch/parse.go` producing
      `Patch{Path, Tag string; Hunks []Hunk}` and
      `Hunk{Line, End int; Old, New []string; Kind}` from the two forms that
      measured best: `PUT N.=M:` followed by a `-` row and a `+` row, and
      `SUB N:` followed by `-old` and `+new`
- [x] Refuse instead of guessing. Return a typed error that names the problem
      for a reply that will not parse, a hunk missing its `-` or `+` row, and a
      reply carrying more hunks than the caller asked for
- [x] Implement the doubling rule, which writes a literal leading `-` or `+` in
      content twice. Test the case no repair can catch: content whose first
      character is a dash, where dropping that dash still parses and silently
      loses it
- [x] Write `internal/patch/repair.go` with the two measured repairs, each
      switchable on its own. Accept body rows that all lack their leading `+`,
      provided none of them starts with `-` or `+`. Re-indent a single-line
      replacement from the line it replaces, except where whitespace matters
- [x] Write table-driven tests for both repairs, including the Python case where
      re-indenting must be refused

> **Completed** 2026-08-19 21:00 UTC
>
> - `make check` — 411ms

## Iteration 3: resolve, apply, refuse

- [ ] Write `internal/edit/resolve.go`. Recompute the tag and compare it
      exactly. On a mismatch, check whether the file is byte-for-byte the
      snapshot text. Name the correct anchor only in that case. Otherwise show
      the current lines and require a fresh read
- [ ] Refuse an anchor no read in this session issued, and refuse any line
      outside `Snapshot.Lines`. Give each its own error
- [ ] Write `internal/edit/apply.go`. Resolve the anchor, apply the hunks in
      memory, convert seven dash variants, eight quote variants and thirteen
      space variants to ASCII, and return the result without writing to disk
- [ ] Return three things from a refused edit: the error, the text the edit
      would have produced, and the file as it stands. SWE-agent's ablation is
      the argument for all three. Without the error the model misdiagnoses;
      without its own attempt it sends the same edit again; without the current
      file it edits against a memory four turns old
- [ ] Test every refusal path, and assert in each that the file on disk did not
      change

## Iteration 4: build the corpus

Assembling the test data is its own kind of work, and the sizing rule this
project measured says an iteration does one kind. It also has no judgement in
it, so it closes on commands alone.

- [ ] Add `journals/` with a `.gitignore` excluding its contents, and a
      `journals/README.md` naming which harness journals to copy there and why
      they are not committed
- [ ] Write `internal/corpus/distill.go` and a `make corpus` target. It reads
      `journals/*.jsonl` and writes `testdata/corpus.jsonl`, keeping only the
      source journal name, the patch form, the line number, the original line,
      the wanted line, the reply text and the recorded verdict
- [ ] Make the target additive. Regenerate the records of every journal present
      in `journals/`, keep the records of every journal that is absent, and
      write the result sorted so two runs over the same inputs produce identical
      bytes
- [ ] Record each source journal's name, SHA-256 and record count in a header
      line. Refuse to write when a present journal's hash differs from the
      recorded one, or when the merge would drop records, unless given
      `FORCE=1`. These two catch a journal that was rescored and a corpus about
      to lose data
- [ ] Test the fresh-clone case directly: with `journals/` empty, `make corpus`
      must leave `testdata/corpus.jsonl` byte-identical. The gitignore creates
      that case, and it is the one that would otherwise destroy the corpus
- [ ] Commit `testdata/corpus.jsonl` and write `testdata/README.md` saying the
      file was extracted from those journals rather than written by hand

## Iteration 5: replay the corpus and settle the disagreements

```backlog
required_acks:
  - disagreements-adjudicated
```

This iteration carries the one ack, because deciding which side of a
disagreement was wrong is a judgement and not a command.

- [ ] Write `cmd/replay-edits`, which reads the corpus and reports per patch
      form how often the applier's outcome matches the recorded verdict
- [ ] Add `go test ./internal/edit -run TestAgainstCorpus`, which requires
      agreement on every record. Keep a checked-in list of settled exceptions,
      each with one line saying which side was wrong and why
- [ ] Settle every disagreement the first run reports. For each, record whether
      the applier or the scorer was wrong, and fix that side

## Iteration 6: the tool surface

- [ ] Expose the applier as the function the agent loop will call:
      `Apply(ctx, Snapshot, Patch, Options) (Result, error)`, where `Result`
      carries the new text, the diff, and the refusal if there was one
- [ ] Add `ratchet edit --file --patch` as a thin command over it, so a person
      or a shell test can drive the applier without an agent
- [ ] Write `docs/edit-applier.md` recording what was built, which measurement
      each decision rests on, and every place the code departed from
      `../docs/architecture.md`. That document is design; this iteration is the
      first evidence about it

## Divergences so far

Not an iteration: no checkboxes, so `backlog` reads this as prose. Where the
code departs from a todo above, the departure is recorded here as it happens.
Iteration 6's last todo folds this into `docs/edit-applier.md`, which is where
it belongs once there is a document to hold it.

- Iteration 2's todo asks for a doubling rule that "writes a literal leading `-`
  or `+` in content twice". The parser does not double, because the format that
  was measured does not either. The probe's own prompt said "`- item` becomes
  `+- item`", which is a sigil followed by a literal dash, and its scorer
  accepted that. Doubling would have produced a parser disagreeing with every
  reply in the corpus. The name is wrong here and in the harness that measured
  it. Renaming it there changes the prompt's hash and so breaks comparison with
  every run that used it, which is why it waits, and why it is tracked with that
  harness rather than here.

- Iteration 2's todos ask for two repairs. There is one. Filling in a missing
  sigil was measured on `put_sigil`, whose body rows are all additions, so a
  bare row there can only mean one thing. This parser reads a form with an old
  row and a new row, and a body of bare rows does not say which is which;
  inferring it would be the guess the old-line check exists to prevent, and
  `put_sigil` was rejected for corrupting thirty times as often. The 64% to 99%
  figure belongs to a form Ratchet does not use. A corrective turn covers the
  same failure at a price that is known: it recovers 221 of 494 diagnosed
  failures and turns 35 into wrong ones. The repair's price is not known,
  because guessing which row is which lands silently.

  If some model's replies in this form arrive all-bare often, the form is wrong
  for that model and qualification should choose another one for it. A repair
  there would hide a model-and-form mismatch that qualification exists to find.

- The re-indent repair runs when a patch is applied, not when it is parsed: it
  needs the line being replaced, and only the applier has read the file. It is
  written and tested and nothing calls it until iteration 3.

- Iteration 1's todo asks for `Snapshot{... Lines map[int]bool}`. The code uses
  `map[int]struct{}`, because a bool implies `false` means something. The todo
  is closed, so it stays as written; iteration 6's documentation todo is where
  the departure belongs.
