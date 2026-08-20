# The edit applier

What the applier does, and the measurement each decision rests on.

[architecture.md](architecture.md) says how it is shaped and is kept true as
that changes; this holds the evidence under it, which is longer than a design
document should carry and would go stale in a different way. Where the plan that
built it was departed from, [the spec](../specs/001-edit-applier.md) records
that.

## What it is

```go
edit.Apply(ctx, reads, patch, current, opts) (Result, error)
```

Given a patch, the file it addresses, and a record of what reads have served, it
returns the file that would result — or refuses and changes nothing. It never
writes. `internal/edit` is checked for imports that could reach a file, so that
is a property of the package rather than a habit of its callers.

Four packages:

| Package               | Holds                                                      |
| --------------------- | ---------------------------------------------------------- |
| `internal/anchor`     | the tag, a snapshot of what a read served, what was issued |
| `internal/patch`      | the two accepted patch forms                               |
| `internal/edit`       | resolve, apply, refuse, diff                               |
| `internal/dev/replay` | the same replies through both, to see where they disagree  |

`ratchet-dev read` and `ratchet-dev apply` drive it from a shell.

## What each decision rests on

**Two forms, `PUT N.=M:` and `SUB N:`, each with a `-` row and a `+` row.**
Eleven were measured at n=400. `put_diff_checked` scored 328 correct with **2
wrong**; `put_diff`, identical except that it does not check the `-` row, scored
316 with **75 wrong**. Checking is free on both axes. The same pairing holds for
`put_oldnew` against `put_noindent`, 8 wrong against 71.

**Refusing rather than guessing.** A refusal costs a corrective turn, which
recovers 221 of 494 diagnosed failures and turns 35 into wrong ones — about 45%
of a correct edit. A wrong landing costs the file and review does not reliably
catch it. So the arithmetic favours refusing anything ambiguous by a wide
margin, and every return other than a complete patch names what is wrong.

**One tag per file, not one per line.** A per-line scheme cost the weaker two of
four models 15 and 19 correct answers in 30, where a file tag cost nothing at
all, 83/90 either way. Its one advantage, catching a slipped line number when
the model copies the right line's hash, fired once in 120 attempts against 41
refusals a file tag never causes.

**An anchor must have been issued, not merely be correct.** A refusal message
names the file's current state, so an identifier can be lifted from it and sent
back without the content behind it ever being read — and every check downstream
passes, because the identifier is correct about a file the model has not seen.
Measured, a local model does this on roughly one refusal in fifteen, and the
edits it produced would have deleted a constructor's `super(id);` and half of a
two-line statement.

**Only one refusal branch may name a replacement anchor.** Where the file is
still the one that was tagged, a mismatch can only be a transcription error and
the resolver knows what was meant. Where the file moved, the same courtesy
points the model at content it has never read. The two are indistinguishable
from the anchor alone, so the decision belongs to the tool.

**No repairs, though two were measured.** Both were measured on a form with no
old row to check, and checking one retires them.

Filling in a missing sigil needs a body whose rows can only mean one thing, and
this form has an old row and a new row. Re-indentation could never fire: of 240
recorded replies whose replacement lost its indentation, the `-` row had lost it
in **all 240**. A model does not slip on one row and keep the other, so the
old-row check refuses those replies before a repair reaches them.

That is a third thing checking the old row buys, beyond being free on
correctness and free on refusals. Those replies get a refusal naming the line
instead, and a corrective turn recovers about 45% of diagnosed failures.

**No similarity matching, ever.** aider ships an edit-distance matcher at
threshold 0.8 and disabled it with a bare `return`. An edit applied to code that
merely resembles the target is a corruption that survives review.

**A hunk limit, from the request rather than the grammar.** A reply carrying
more changes than were asked for is well formed and edits lines nobody
mentioned. Asking two models for two hunks produced replies with 27, 57, 59, 68
and 71.

**A code fence is skipped outside a hunk and refused inside one.** 221 of 3,789
recorded replies arrive fenced, and the harness that scored them matched its
patterns anywhere in the text, so every published rate for these forms already
assumes the fence is ignored; 81 of those were scored correct. Inside a hunk it
is refused, because a bare fence between two `+` rows would shorten the
replacement and leave an unterminated fence in the file. None of the 221 has a
body row after a fence.

## What this does not establish

**Four refusals cannot be reached from any recorded reply.** An unread path, a
moved file, a windowed read, and an anchor the tool never issued all need
something only an environment can do. No model has ever mistranscribed an anchor
— 3,788 of 4,099 recorded attempts wrote a section header and all 3,788 carried
the tag they were served — so those paths are covered by tests in
`internal/edit`, where the setup is visible, and not by evidence.

**The replay agrees with the harness and not with the truth.** Both were written
from the same measurements, so agreement means they made the same reading, not
that the reading is right. Where they differ, one is wrong: two such differences
were found and fixed, one on each side. The rest are recorded with the side that
was wrong, because correcting them means the harness stops being a pattern
matcher, which would change every number it has published.

**Nothing here has run against a model.** Every number above comes from replies
recorded by the harness. The applier has never been called by an agent loop, and
the first thing that does will be evidence about this document.
