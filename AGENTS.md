# Ratchet

Ratchet runs a local model against a spec until the spec's own gates pass, and
refuses to lie about whether they did.

**This file is an index. Keep it that way.** Nothing belongs here that is not a
pointer or a rule short enough to fit on one line. A context file that grows
into a manual stops being read, and the things in it stop being followed.

`CLAUDE.md` is a symlink to this file, so both names find it.

## Read this before

| Doing                        | Read                                          |
| ---------------------------- | --------------------------------------------- |
| anything                     | [docs/conventions.md](docs/conventions.md)    |
| changing how it is built     | [docs/architecture.md](docs/architecture.md)  |
| changing what it does        | [docs/product.md](docs/product.md)            |
| touching the applier         | [docs/edit-applier.md](docs/edit-applier.md)  |
| picking up work              | [specs/](specs/), lowest open iteration first |
| noting an idea, not doing it | [specs/TODO.md](specs/TODO.md)                |

## Rules that fit on one line

- Work one iteration at a time, in order. Close it before starting the next.
- `make check` is the gate. It must pass before an iteration closes.
- Make every new gate fail on purpose once before trusting a pass from it.
- Never commit or push without asking. Approval of one commit is not approval of
  the next.
- A commit body says why, not what. Draft it, review it, then ask.
- A design doc describes the target. When the code contradicts it, correct the
  doc; the spec is where a departure from the plan is recorded.
- Cite the measurement, not the file path: paths outside this repo will break.

## What is enforced rather than trusted

`go test ./internal/convention` checks the repo's own source for the conventions
that can be checked: table tests using subtests, and doc comments on exported
identifiers. Both are in `make check`.

The reason is in that package's own comment. A trap recorded in a code comment
was shipped anyway a day later, and cost one model 0 of 50 on a probe. Write the
convention down, then enforce it where possible.
