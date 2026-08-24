---
status: active
created: 2026-08-22T00:00:00Z
updated: 2026-08-24T06:24:37.327966998Z
required_commands:
  - cmd: make check
---

# Ratchet 005: one iteration, end to end

Nothing here runs a model. The applier refuses well, the session holds reads
across turns and writes when an edit resolves, and no code path reaches a model
host. So every claim in `docs/architecture.md` past the tool boundary is
untested: the loop's shape, the error classes, what a real model does when
handed these tools and these refusals.

The queue calls this the thing every downstream decision is guessing about, and
it is right. The refusal economics the design rests on were measured in a
research harness against a different tool surface. Whether they survive contact
with this one is unknown.

This spec builds the thinnest path that runs a model against a real iteration
and stops: a provider, a dispatcher, and a loop around them.

## What this spec is not

- **No gates, no commits, no snapshots.** The loop stops when the model says it
  is done. Whether it is done is the runner's question and the next spec's.
- **No retry, no attempt budget, no escalation ladder.** One attempt. A failure
  ends the iteration and says why.
- **Not all seven tools.** `read`, `edit`, `done` and `blocked`. Enough to
  finish an iteration that edits an allowed file, and enough for the executor to
  stop on purpose. `write`, `bash` and `revert_file` wait.
- **No container.** The session's root is a directory. Sandboxing is separate
  and the architecture treats it separately.
- **One provider.** Ollama's native `/api/chat`, because that is the host the
  measurements ran against and the only one whose lies are documented.

## Decisions

**The provider interface is the seat-agnostic one already designed**, so the
drafter can be handed a different implementation later without the loop knowing.
`Stream` and `Allocated`, nothing else.

**`Allocated` is not optional and is checked before the first turn.** Ollama's
OpenAI-compatible route silently discards `options.num_ctx`, so a model asked
for 48k loads at 4096 and nothing reports it. The native route plus `/api/ps`
reports the real number. A loop that starts without checking is measuring a
context it does not have. This is a recorded lie, not a precaution.

**One tool call per turn, enforced by having no batch form.** A reply carrying
two calls is a protocol error, not a queue. A partially applied multi-edit is
not a state this system can represent, and the schema is where that is
guaranteed.

**Advertise the strict schema, accept the permissive union.** The model is shown
one shape. `execute` accepts `path` or `file_path`, a bare string where an array
is declared, `"3"` where a number is. Advertising the loose form teaches
sloppiness; accepting only the strict form discards calls whose intent is not in
doubt.

**Every refusal the tools already produce reaches the model unchanged.** The
applier's refusal text is measured work and the loop is a pipe for it, not an
editor of it. A loop that summarizes a refusal throws away the part that was
measured.

**The idle timer resets per token, not per request.** A stream that is producing
is alive. A model thinking for four minutes and a dead socket look identical to
a request-level timeout, and one of those should be waited for.

**A tool error is a turn, not a failure.** `E_FILE_NOT_ALLOWED` and the rest go
back as the tool's result and the model gets another turn. Only a protocol
failure or the model stopping ends the iteration.

## What this must demonstrate

A local model, given an iteration naming one file and one change, drives itself
to `done` through `read` and `edit` against a real host, and the file on disk
holds the change.

That is the whole bar. Not that it does so reliably, not at what rate, not on
which models. Those need this to exist first.

## Iteration 1: the provider

- [x] `internal/agent` with the `Provider` interface: `Stream` and `Allocated`
- [x] An Ollama implementation over the native `/api/chat`, streaming, with the
      idle timer resetting per token
- [x] `Allocated` reads the real context from `/api/ps` and the loop refuses to
      start below what the iteration declares it needs
- [x] A scripted provider for tests, so nothing below needs a host

> **Completed** 2026-08-24 06:29 UTC
>
> - `make check` — 1.4s

## Iteration 2: the tools the executor is given

- [x] `read`, `edit`, `done` and `blocked` as dispatchable tools over the
      existing session, each advertising a strict JSON Schema
- [x] `execute` accepts the permissive union and says which spelling it accepted
- [x] Two calls in one reply is a protocol error naming both
- [x] Every existing refusal reaches the model as the tool's result, unedited

> **Completed** 2026-08-24 08:57 UTC
>
> - `make check` — 1.5s

## Iteration 3: the loop

- [ ] One iteration, one attempt: assemble the prompt, stream, dispatch one
      call, append the result, repeat until terminal
- [ ] `done` and `blocked` are the only terminal verbs, and both are named in
      every failure message the executor sees
- [ ] A tool error is a turn; a protocol failure ends the iteration and says why
- [ ] Run it against a host, on a spec iteration that edits one file, and record
      what happened

## Where this plan was departed from

**`edit` takes the patch as text, not the four arguments the design names.** The
design writes it `edit(path, anchor, end?, text)`, which has no room for the
text being replaced. Stating the old row is what took corruption from 75 in 400
to 2, and the refusals this spec exists to pipe through are the ones the parser
and the applier produce against that notation. So the tool takes the section
header and hunks as written, and the parser and applier are reused whole rather
than reimplemented over structured arguments.

**Dispatch lives in `internal/agent` and the tools in
`internal/executor/tool`.** The loop holds a `Dispatcher` interface, and the
seat's tools implement it, so the loop never learns which seat it is running and
neither package imports the other in a circle.
