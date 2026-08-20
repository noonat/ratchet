# Journals

Raw output from the measurement harness that preceded this repo. Copy the files
named below in here, then run `make fixtures`.

Nothing here is committed. A journal is 8 to 76MB, almost all of it the prompt
text sent to the model, and the applier reads none of it. What it does read is
distilled into `testdata/fixtures.jsonl`, which is committed. So a fresh clone
has the fixtures and not the journals, and `make fixtures` is written to survive
that.

## What to copy

| Journal                   | Why                                              |
| ------------------------- | ------------------------------------------------ |
| `edit-candidates.jsonl`   | four models, both original fixtures              |
| `edit-conclusive.jsonl`   | the eleven-form matrix, four models              |
| `edit-sub-v2.jsonl`       | the substitution forms after the prompt was cut  |
| `edit-gptoss-4k.jsonl`    | one model at a cap high enough not to bind       |
| `edit-think-low-4k.jsonl` | the same two models at the lowest thinking level |
| `edit-ts-md-4k.jsonl`     | TypeScript and Markdown, which nothing else had  |

The first five names came from the harness and are kept as it wrote them, so a
journal here can be matched against the run that produced it. They are not good
names: `candidates`, `conclusive` and `sub-v2` say where a run sat in a sequence
of attempts rather than what it measured. `gptoss-4k` and `think-low-4k` name a
condition and have held up, which is the pattern the sixth follows and the one
to use from here. `specs/TODO.md` carries the rename, which waits on this repo
generating its own journals.

## What to leave out, and why

**`edit-think-low.jsonl`.** Superseded, and `make fixtures` refuses it rather
than trusting this paragraph.

`edit-think-low-4k.jsonl` reran the same cells with the same seed at a higher
cap. For `gpt-oss:20b`, 498 of its 500 replies are identical across the two
files, so keeping both counts almost every one of them twice.

The higher cap did not fix everything. `north-mini-code-1.0` still ran out of
room on 381 replies at 4096, against 441 at the old cap, so its records measure
the cap in both files. They are kept because a reply that overran is still a
reply the applier has to refuse, but they say nothing about the form.

**Journals from any probe other than `edit`.** A distilled record is one line,
its original text and its wanted text. The other probes record a different
shape: `multihunk` addresses several lines at once, `collide` and `straddle`
record a delimiter position, `compete` and `envelope` score something other than
an edit. Their records cannot be replayed against a single-line expectation, so
the distiller skips them rather than storing a record it cannot check.

**Forms this repo does not parse.** The harness measured eleven patch forms. Two
are implemented here, `put_diff_checked` and `sub_diff`, and a reply in any
other form would be refused for its syntax rather than for anything worth
learning. The distiller keeps those two.
