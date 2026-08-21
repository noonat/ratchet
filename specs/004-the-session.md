---
status: done
created: 2026-08-21T07:30:00Z
updated: 2026-08-21T07:21:48.909483569Z
required_commands:
  - cmd: make check
---

# Ratchet 004: a session that outlives one command

The applier is a library nothing calls. `anchor.Reads` is built per call and
filled with the file about to be edited, so the provenance rule cannot fail:
every anchor was served a statement earlier. It earns its keep when a read is
three turns old.

Nothing writes either, so a caller gets the edited text and no way to save it.

This spec adds both halves. A session keeps what reads served across calls, and
an edit writes to the file when it resolves.

## What this spec is not

- No agent loop, no model client, no container, no journal. This is the surface
  a loop will call, not the loop.
- No write root, and no `write` tool. A scoped root is the drafter's problem and
  belongs with the seat that needs it; this is the executor's anchored `edit`.
- No new behavior in `internal/edit`. Its import allowlist is the reason the
  write lives elsewhere: the test says the list "is the whole of what the
  applier may reach; writing belongs to whatever calls it", and that separation
  is what lets every refusal path be proven to touch no file.
- No persistence. A session is in memory for one process; surviving a restart is
  the journal's job and the journal is not here yet.

## Decisions

**The session is a new package, because the applier may not reach a file.**
`internal/edit` is fenced by an import allowlist that admits no filesystem.
Adding a write there would either break that fence or hide behind it.
`internal/executor/tool` holds the session and the two calls a loop makes, and
depends on `edit` rather than the reverse. The architecture already puts the
executor's tools there, and says why: the two seats' tool sets differ and
neither is general.

**One write per patch, after every stage that could refuse.** Resolve, splice in
memory, then write once. `internal/edit`'s package comment already claims a
refusal cannot half-apply; writing per hunk would make that false for the first
time. The rejected alternative is streaming each hunk as it applies, which is
cheaper on memory for files nobody in this corpus edits and turns a refusal into
a corrupted file.

**A replaced file keeps its own mode.** Write a temporary file beside the
target, close it, then rename. The distiller does this already and sets 0644,
because it owns the file it writes. A source file belongs to the repository, so
its mode comes from `Stat` on what is being replaced, since `CreateTemp` makes
0600 and a rename carries that over, so every edit would otherwise leave an
edited file at 0600.

**A successful edit records a fresh snapshot and hands back its tag.** The file
just changed, so the anchor the model holds is stale, and a second edit to the
same file would be refused for a reason the model cannot act on. Returning a
live tag is also the measured flow, and it is why provenance exists rather than
tag validation alone. A successful edit puts a live tag back in circulation, so
validity alone would accept a tag the model had never read.

**A read records what it displayed, not what it stamped.** The whole file for
now, which is what every read in the corpus did. `anchor.NewSnapshotForLines`
stays reachable because a windowed read is a listing over part of a file with a
tag over all of it, and the line set is what stops an edit outside the window.

## Iteration 1: the session records reads and writes edits

Files: a new `internal/executor/tool` package with `session.go`, `paths.go` and
`session_test.go`, and `cmd/ratchet-dev/apply.go` for a flag that exercises the
write from a shell.

- [x] Add `internal/tool` with a `Session` holding a `*anchor.Reads` and the
      root it resolves paths against. Document why it is not in `internal/edit`
- [x] `Session.Read(path)` renders the tagged, numbered listing and records the
      snapshot, so a later edit can satisfy provenance
- [x] `Session.Edit(patch)` calls `edit.Apply` and, only when it resolves,
      writes the result
- [x] The write is a temporary file beside the target, renamed over it, carrying
      the mode of the file it replaced
- [x] A resolved edit records a fresh snapshot for the path and returns its tag
- [x] Table tests: an edit against a path this session never read is refused,
      which is the provenance rule failing for the first time in a real path
- [x] A read, then two unrelated reads, then an edit against the first path
      still resolves, which is the case a per-call `Reads` cannot express
- [x] A refused edit leaves the file byte-identical, asserted for a stale
      anchor, a line outside the read, and a mismatched old row
- [x] A file whose mode is 0600 keeps it, and one whose mode is 0755 keeps that
- [x] The tag returned by a resolved edit satisfies the next edit to that file,
      with no read in between
- [x] Run the provenance test before `Session` exists and see it fail for the
      right reason: nothing refuses today because nothing records
- [x] `ratchet-dev apply --write` lands the result, so the path is exercisable
      from a shell rather than only from tests
- [x] Record the session in `docs/architecture.md` if it is not already there,
      per the rule that a design document describes the target

**Gate:** `make check`

> **Completed** 2026-08-21 07:21 UTC
>
> - `make check` — 1.4s

## Where this plan was departed from

The package sits at `internal/executor/tool`, not `internal/tool`. The plan
named a top-level package; the architecture already names this one and gives the
reason, which is that each seat's tools nest under it because the sets differ
and neither is general. Writing the last todo, the one that records the design,
is what surfaced it.

`Session.Edit` returns a `tool.Result` holding `edit.Result` in a named field
beside the new tag. The plan said the call would hand back a tag, and the
applier never writes, so the tag belongs to the package that has a file to
stamp.

`Session.Preview` was added, which the plan did not name. Without it the dev
command had to reach around the session to apply without writing, which left two
paths through the applier and a pair of variables filled by whichever branch
ran. A preview is a different operation rather than a flag on this one, so it is
a second method over one implementation.

`edit.Result` was split, which touches `internal/edit` after this spec said it
would not. It held two renderings of the same file that were never both set:
`Would`, the loose splice a refused edit would have produced, and `Now`, the
file before the edit. Both are what a refusal owes the model, per the ablation
argument the package comment already made, so both moved onto `Refusal` as
`RefusedText` and `Text`. `Result` is now `Text` and `Diff`, every field always
set. No behavior moved, and a caller that ignores the error can no longer
mistake a loose splice for a file to write, because reaching it means unwrapping
a refusal.

`ratchet-dev apply` takes the address from `--file` rather than from the reply's
header. A session resolves an address under its root, and the header carries
whatever path the reader was given, absolute included, which a root cannot
contain. The header still says which file the model thought it was editing; it
no longer says where to write.
