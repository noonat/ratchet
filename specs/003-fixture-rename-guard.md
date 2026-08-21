---
status: done
created: 2026-08-20T21:05:00Z
updated: 2026-08-21T06:08:25.274066909Z
required_commands:
  - cmd: make check
---

# Ratchet 003: the fixture rebuild names a rename

The fixture rebuild is additive by design: records of a journal present in
`journals/` are regenerated, records of every journal absent are carried over,
and the file is committed. That works until a journal is renamed or copied
outside the repo. The new name arrives with an unmatched hash, the old name is
absent, so the rebuild keeps the old records and adds the new ones on top.
**Every reply that journal contains is now counted twice**, and nothing at
rebuild time says why.

Something does catch the doubling, but later: a doubled set replays every
settled mismatch twice against a fixed count, so `make check` fails as a count
mismatch nobody can read back to the rename. The check moves that moment to the
rebuild, where the cause names itself. The distiller already hashes both sides —
the check is one comparison.

Spec **002** is the other half of the same observation, on the applier's side:
refusals that say why. This one is about `make fixtures` refusing what it should
refuse.

## What this spec is not

- No change to the distiller's keep/skip rules, the forms it reads, or the sort
  and the reproducible write. A rebuild over the same inputs still produces the
  same bytes.
- No new flag. `FORCE=1` keeps doing what it does on the other four rebuild
  refusals. This one does not accept it, in any of its three cases.
- No edits outside `internal/dev/fixture`, other than that one flag-usage line
  in `cmd/ratchet-dev/main.go`, which names the refusals the flag accepts.

## Decisions

**The comparison is keyed on the hash, and runs two ways.** Per journal present,
its hash is compared against (a) the recorded hash of every _other_ recorded
source, and (b) the hash of every journal earlier in the same run. Part (a) is
the rename of, and the copy of, a journal already in the file; part (b) is the
copy of a journal not yet rebuilt — two new names with the same content pass
every existing check (hash unmatched on both, the count grows, the total grows),
so without it they are both accepted and the doubled set is committed. Part (a)
runs before the same-named hash-change check, so the more specific refusal — the
one naming the other journal — is the one returned. When the hash matches two
candidates, the first by name is the one the message names — under SHA-256 they
are the same file.

**The refusal names both journals, says keeping both counts every reply twice,
and names the remedy, which a single fact selects: whether the present name is
recorded under _this_ hash, and whether the other name is present.**

- The present name is recorded under this hash, and another name is recorded
  under it too: the file already holds both names under one hash. Keep one name
  — remove the other's records and its source line from the committed fixtures
  file, and its file if it is still present — and rebuild.
- The present name is not recorded under this hash, and the other name is
  present as well: two identical files. Keep one file, remove the other, and
  rebuild.
- The present name is not recorded under this hash, and the other name is
  absent: this is a rename or a copy. Recorded under a _different_ hash is still
  this case — the file then holds the two names under two hashes and the state
  is a copy of the recorded name's content, not a doubled header. Rename the
  file to the recorded name, or remove it, and rebuild.

`force` does not remove a duplicate in any of the three; the remedy is to remove
the duplicate, not to accept it.

**This refusal is not forceable, in any of its three cases.** `FORCE=1` exists
on the other four rebuild refusals — superseded, rescored, per-journal shrink,
and the total backstop — because the evidence under a _named_ source genuinely
changed and accepting that may be intended. Here the evidence did not change; it
is doubled, and doubling is not an intended event. A second name on the same
content may even be the product of a legitimate deterministic rerun — 200 of 200
replies in one re-run were byte-identical at temperature 0 — but the replies are
identical either way and are counted twice either way, and the remedy, keeping
one name, is the same. The alternative, accepting `force` in the both-recorded
case alone so a stranded state can keep moving, commits the doubling permanently
and puts `force` on the path of every later rebuild, with the same remedy left
undone.

**The existing `Superseded` tests keep working when the list grows.** Two tests
— `TestRebuildRefusesASupersededJournal`, which then forces a rebuild, and
`TestARefusalIsTypedSoItPrintsAsAMessage` — write identical content to every
name in `Superseded`. Under this check a second entry in that list is an
identical pair, and the first test's forced rebuild would hit the refusal.
Varying the row by name in the two loops keeps them true for whatever list
grows. It costs one word and belongs here rather than in whoever adds the entry.

## Iteration 1: `make fixtures` refuses a second name on the same content

Files: `internal/dev/fixture/distill.go` (the check in `Rebuild`),
`internal/dev/fixture/distill_test.go` (the table tests, sibling of the existing
`Rebuild` refusal tests — the `build` helper is there; `journal` and `row` are
in the neighboring `fixture_test.go` — plus the two `Superseded` loop tests),
and the `--force` usage line in `cmd/ratchet-dev/main.go`.

- [x] In the `Rebuild` loop, after hashing a journal present, compare its hash
      against (a) the recorded hash of every _other_ recorded source and (b) the
      hash of the journals earlier in the same run; when one matches, refuse,
      before the same-named hash-change check
- [x] The refusal names both journals and says keeping both counts every reply
      twice. Which remedy follows is selected by whether the present name is
      recorded under this hash
- [x] Present name recorded under this hash, another name recorded under it too:
      remove the other's records and its source line, and its file if it is
      still present
- [x] Present name not recorded under this hash, other name present: remove one
      of the two files
- [x] Present name not recorded under this hash, other name absent: rename to
      the recorded name, or remove the file
- [x] `FORCE=1` does not override this refusal, in any of its three cases. The
      `force` parameter is ignored for this one; leave it doing what it does for
      the other four
- [x] Table test: a journal under a new name with the same content as a recorded
      source is refused, names the recorded name, and offers the
      rename-or-remove remedy. With `force` true it is still refused
- [x] Two new journals with identical content, neither recorded, are refused and
      name both, with the remove-one-file remedy — the adjacent case a
      present-against-recorded comparison alone misses
- [x] A file that already holds two sources under one hash, rebuilt with one of
      the two names present, is refused with the remove-the-duplicate remedy.
      With `force` true it is still refused
- [x] The same content under two names, both present, one recorded under that
      hash and the other not, is refused and names both, with the
      remove-one-file remedy
- [x] A journal with genuinely new content is not refused, and the merge keeps
      both sources' records
- [x] A journal recorded under a _different_ hash whose hash also matches the
      one another source is recorded under — the case that pins the order
      against the rescored check — is the named-journal refusal with the
      rename-or-remove remedy. The assertion is on the remedy text and not only
      the name, since naming does not separate the branches from each other
- [x] In the two `Superseded` loop tests, vary the journal row by name, so a
      second entry in the list is not an identical pair
- [x] Run the rename test before changing `Rebuild` and see it fail: the rebuild
      should currently succeed with the records doubled, which is the corruption
      this iteration removes
- [x] `cmd/ratchet-dev/main.go`'s `--force` usage line becomes one that names
      what it accepts — the rescored, superseded, shrink, and total-backstop
      refusals — since the named-journal refusal does not accept it
- [x] `TestWriteIsReproducible` and the existing `Rebuild` tests pass, except
      the two `Superseded` loops, which now vary their row

**Gate:** `make check`

> **Completed** 2026-08-21 06:08 UTC
>
> - `make check` — 1.3s

## Where this plan was departed from
