# Test data

`fixtures.jsonl` holds recorded model replies, used to check that this repo's
applier reaches the same verdict the measurement harness reached.

These are replies, not source files. The word means something else in the
harness, where `fixtures/` holds the files being edited. Here a fixture is one
reply: the line it addressed, that line's original text, the text the edit
should have produced, and the verdict recorded at the time.

## Where it came from

Extracted by `make fixtures` from journals the harness wrote, listed in
[../journals/README.md](../journals/README.md). Not written by hand, and not
edited by hand either: a record altered here would assert something no model
ever sent.

The first line is a header naming each source journal, its SHA-256 and how many
records it contributed. Rebuilding refuses if a journal's hash has moved,
because that means it was rerun or rescored and the records built from it would
say something different.

## What is in it, and what is not

Only the two patch forms this repo parses, `put_diff_checked` and `sub_diff`.
Only the `edit` probe, whose records are one line with one expected result.

Four of the five recorded outcomes: `correct`, `refused`, `malformed` and
`applied_wrong`. The refusals and the malformed replies are the more interesting
half, because agreeing about a bad reply is harder than agreeing about a good
one.

`failed` is absent. A failed attempt is one that produced no reply, and a record
with no reply text is skipped because there is nothing to replay. That removed
310 records: every `failed` attempt in the journals distilled so far, and two
`malformed` replies that were empty rather than wrong.

Three of the applier's refusals cannot appear here either. `ReasonNoRead`,
`ReasonMistranscribed` and `ReasonFileMoved` need an anchor a read never issued,
a mistyped anchor, or a file that changed under one. The harness never moved a
file, and across the 2,955 replies in these two forms that got far enough to
write a section header, no model ever mistranscribed the anchor in it. So
nothing here triggers those three, and they are covered by tests in
`internal/edit` instead.
