---
status: done
created: 2026-08-20T21:05:00Z
updated: 2026-08-21T05:39:52.209862565Z
required_commands:
  - cmd: make check
---

# Ratchet 002: the near-miss report

The applier refuses an edit whose old text differs from the file by nothing but
whitespace, and says only that it does not match. The cost of that is measured,
not estimated. **Of the 650 `old_mismatch` records in `testdata/fixtures.jsonl`,
323 differ from the file by whitespace only** (counted by stripping all
whitespace from both sides before comparing), and **of the 102
`fragment_not_found` records, 34 are the same for the fragment** — in each case
the edit was right about everything except whitespace. A diagnosed refusal
recovers about 45% of such failures (`docs/edit-applier.md:42`), because the
model resends the one line as it is rather than guessing at it. The applier's
own package comment in `internal/edit/edit.go` records the 30 of 119 indentation
casualties, from a different population (replies whose replacements mangled
indentation), and says the old-row check caught them rather than repaired them.

So the model gets

```
Line 141 is `if (…) {`, not `      if (…) {`.
```

and has to deduce that it dropped the indent. Saying it is a sentence.

This is a refusal-quality change in `internal/edit`. Spec **003** is the other
half of the same observation ("refusals that say why" and "accepts that should
be refused"): the fixture rebuild.

## What this spec is not

- No fuzzy applying. A whitespace-only mismatch is still refused. The point is
  the sentence in the refusal, not a second application path. The diagnosis
  could be read as a step toward exactly that — a matcher that accepts the old
  row once whitespace is ignored — and the committed fixture is the
  counter-argument: on these 323 records a whitespace-tolerant match would write
  a wrong file. **If the old rows were matched fuzzily, 0 of the 323 replies
  would land correct** (a `+` row byte-equal to the wanted line), 319 are the
  wanted line with its whitespace mangled, and 4 are wrong content. The research
  notes record all three surveyed editors converging on whitespace-only fuzz,
  never similarity fuzz (aider's edit-similarity matcher is deliberately
  disabled), and that is the settled shape this spec leaves alone.
- No new `Reason` values and no new fields on `Refusal`. Callers branch on the
  reason; it is already right here. The model reads the message; that is the
  channel.
- No change to the tag or to what counts as a match. `anchor.Normalize` keeps
  doing exactly what it does.
- No new packages, and no edits outside `internal/edit`.
- The sentence's effect on the recovery rate is not measured by this spec. What
  it leans on is the cost of the refusal it repairs (45%, above) and the shape
  of the near-miss (the line the model should copy is named). If a later run
  shows the sentence moving nothing, the instrument is a probe with two arms
  differing only in the refusal's wording — the same instrument the research
  notes used when a refusal's wording was in question.

## Decisions

**Near-miss means "equal once all whitespace is dropped."** Both sides with
every character `unicode.IsSpace` would drop, compared for equality.
`unicode.IsSpace` is the standard answer in one call; it would also catch a
non-breaking space substituted for an ordinary one — unmeasured here, since none
of the 323 involves non-ASCII whitespace (the only non-ASCII characters in them
are em-dashes: four across two records, three in one line and one in the other)
— so the NBSP test case below is the definition's pin, not a recorded case. The
strings are still not byte-equal; the code only reaches the diagnosis from a
mismatch.

**Per row for PUT, for the fragment for SUB.** The refusal names the first row
of a hunk that does not match (`refuseMismatch` returns at it), and the sentence
must be true of the row it names: **it attaches when that named row is a
near-miss**, whether or not a later row of a multi-row hunk is a content
mismatch. It does not attach when the named row is a content mismatch, even if a
later row is whitespace apart — the sentence sends the model to copy the row it
is pointed at, and a claim licensed by a different row in the same hunk would be
false about the row named. A SUB fragment gets it only when the fragment is
absent and occurs exactly once on the whitespace-stripped line. It cannot attach
to a fragment that is present at all: each byte-exact occurrence survives the
strip as a non-overlapping occurrence, so one that appears twice necessarily
appears twice stripped, and `strings.Count` never lands on one for it. (A review
of this spec proposed "exact count two, stripped count one" as reachable; a
brute force over fragments and lines up to length twelve found no example, and
it is unreachable in general.)

**One sentence, and it says whitespace, not indentation.** 318 of the 323 differ
in their leading whitespace, 5 differ in an internal space, and none differs in
trailing spaces alone. Naming the indentation would name closer to the fix in
most cases, but the fix the sentence points at — copy the line as it stands — is
already in the message and is the same fix in the five that are not indentation.
Two message shapes for one action is not a win, so the sentence stays generic.

**A fragment that strips to nothing is never a near-miss.** In Go,
`strings.Count(s, "")` is `len(s)+1`, so an all-whitespace fragment would be
counted as occurring exactly once on a line that also has no content — the one
line it can match, and the one where "appears once" is false. The count is
guarded by requiring the stripped fragment to hold at least one character.

**The sentences.** The existing refusal prose is designed, so the additions are
fixed here rather than left to the implementer. The anchors are quoted exactly
as they stand in `internal/edit/apply.go`:

- PUT, `apply.go:127` — the refusal ends with
  `Re-read the file, or send the edit again stating the line as it actually is.`
  Insert ` The difference is whitespace only.` before that sentence.
- SUB, `apply.go:147` — the refusal ends with
  `A fragment has to appear exactly once.` Append
  ` Once whitespace is removed, it appears exactly once.` after that sentence.

**`unicode` is allowed into `internal/edit`'s import list.** The allowlist in
`TestNothingHereCanReachAFile` is the deliberate-act mechanism the test's own
comment names; `unicode` can open no file, and editing the list is that act.

## Iteration 1: the applier says when a mismatch is whitespace only

Files: `internal/edit/apply.go` (the two refusal messages, one small helper),
`internal/edit/write_test.go` (the allowlist entry), one new
`internal/edit/apply_near_miss_test.go` (the new tests, tabled per
`docs/conventions.md`).

- [x] Add a helper `whitespaceOnly(a, b string) bool` in `apply.go`: true when
      `a != b` and the two are equal once every `unicode.IsSpace` rune is
      removed. Document what it is for in a comment, per the doc-comment
      convention
- [x] In `refuseMismatch`, when the named row — the first row that does not
      match — is a near-miss, insert ` The difference is whitespace only.`
      before that message's final sentence. When the named row is a content
      mismatch, it does not, even if a later row of the hunk is whitespace apart
- [x] In `substitute`, when the fragment is absent on the row, its stripped form
      is not empty, and it occurs exactly once on the whitespace-stripped line,
      add ` Once whitespace is removed, it appears exactly once.`
- [x] Add `"unicode"` to the allowlist in `TestNothingHereCanReachAFile`
- [x] Table tests in `apply_near_miss_test.go` for the measured shapes: a PUT
      row that lost its indent (318 of the 323), and one whose internal spacing
      differs (5 of them), are refused with the diagnosis
- [x] The two definition pins: a row that gained trailing spaces, and one whose
      single differing space is a non-breaking space (U+00A0). No record differs
      in trailing spaces alone and none holds non-ASCII whitespace, so each pins
      what `unicode.IsSpace` covers rather than a recorded case
- [x] The per-row rule, on a two-row PUT hunk: a first row that matches with a
      second differing by an indent is refused with the diagnosis, and the same
      hunk with the content mismatch first is refused without it. The parser
      accepts multi-row ranges (`internal/patch/parse.go`) and the corpus holds
      none, so this pins the rule on a grammatical shape
- [x] The SUB cases: a fragment absent byte-exact and occurring exactly once
      once whitespace is dropped gets the sentence, and one occurring twice
      exactly gets none
- [x] The empty-fragment guard: an all-whitespace fragment gets none, pinned on
      a line that also strips to empty — the case an unguarded `strings.Count`
      would match
- [x] A content mismatch, not whitespace, still refuses and gets neither
      sentence
- [x] Every case above still refuses: assert the outcome is a refusal, not an
      application, in each
- [x] Run the new tests before changing `apply.go` and see them fail on the
      message, then pass after
- [x] `TestAgainstFixtures` still passes: the replay compares recorded outcomes
      against replayed ones, and this sentence enters the detail message rather
      than the outcome
- [x] `TestEverySettledDecisionCoversWhatItSays` still passes: the settled
      decision at `internal/dev/replay/settled.go:87` matches its text as a
      **substring** of the detail (the `Because` field, via `strings.Contains`
      in `Explain`), and the sentence enters before the final one, leaving that
      substring intact

**Gate:** `make check`

> **Completed** 2026-08-21 05:39 UTC
>
> - `make check` — 1.3s

## Where this plan was departed from

The new tests went into `internal/edit/apply_near_miss_test.go`. The plan named
`internal/edit/nearmiss_test.go`, which said what the tests were about and not
what they test; the name that ships pairs with `apply.go`, where the two
messages and the helper live.
