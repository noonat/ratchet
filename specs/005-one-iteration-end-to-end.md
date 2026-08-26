---
status: done
created: 2026-08-22T00:00:00Z
updated: 2026-08-26T03:27:56.061702715Z
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

- [x] One iteration, one attempt: assemble the prompt, stream, dispatch one
      call, append the result, repeat until terminal
- [x] `done` and `blocked` are the only terminal verbs, and both are named in
      every failure message the executor sees
- [x] A tool error is a turn; a protocol failure ends the iteration and says why
- [x] Run it against a host, on a spec iteration that edits one file, and record
      what happened

> **Completed** 2026-08-25 06:08 UTC
>
> - `make check` — 1.4s

## Iteration 4: carry what a reply arrives with

The loop keeps a reply's text and its tool calls and discards everything else.
Two defects follow.

A reasoning model that hits the output cap returns empty text, no tool calls,
and a stop reason of `length`. The loop reports "the reply carried no tool
call", which blames the model's tool-call format for a truncation. `ollama.go`
keeps `Thinking` for this case and `Message` has nowhere to put it, so it is
dropped one line later.

The last-turn warning is written and never sent. `result` computes
`left = max - turn` for the turn that just ran, so the `left == 0` branch fires
on the final turn, whose tool result is appended after the loop ends. Nothing
follows it. The model's actual last turn is told "1 turns left", which is also
the wrong plural.

- [x] Add `Thinking` and `Done` to `agent.Message` and set them where the
      assistant turn is appended, so the journal holds what arrived rather than
      what parsed
- [x] When a reply carries no tool call and the stop reason is `length`, say the
      reply hit the cap and give the cap
- [x] Fire the last-turn warning when one turn remains, so it reaches the model
      while it can still act, and make the count read correctly at one
- [x] Break both: a reply with `Done: "length"` and empty text must not report a
      missing tool call, and a four-turn run must deliver the last-turn warning
      as the fourth turn's input

> **Completed** 2026-08-25 07:12 UTC
>
> - `make check` — 1.4s

## Iteration 5: make an unusable argument cost a turn, not the run

`Execute`'s doc comment states the rule: a refused call is an Outcome because
the model gets another turn, and an error means another turn would not help.
Argument handling does the opposite.

`text` returns an error when it finds no spelling it recognises, `Execute`
passes it up, and the loop ends the run. A model that finishes the work and
calls `done` with no summary therefore loses the run, and finished work is
recorded as a protocol failure. Another turn would fix it: the model is told
what is missing and calls again.

`text` also commits to the first key that is present rather than the first that
holds a value. Given `{"path": null, "file_path": "a.ts"}` it fails on `path`
and never tries the spelling carrying the value. Hosts produce that shape when
they fill declared-but-unset arguments.

- [x] Fall through to the next spelling when a value is present and unusable,
      and report a failure only when no spelling yields a string
- [x] Return a missing or unusable argument as an Outcome naming what was wanted
      and what arrived
- [x] Keep an unknown tool name an error, because no turn makes it exist
- [x] Break both: `done` with empty arguments must cost one turn and not the
      run, and `{"path": null, "file_path": "a.ts"}` must read the file

> **Completed** 2026-08-25 15:02 UTC
>
> - `make check` — 1.5s

## Iteration 6: enforce the file list the prompt promises

`System`'s doc comment states the rule: every rule in the prompt is one the
tools enforce anyway. `Task` breaks it. It tells the model which files it may
touch, and nothing checks. `NewSession` fences to a root, the file list never
reaches it, and a model editing a different file under that root is told
`applied.`

The architecture already names the refusal this should produce:
`E_FILE_NOT_ALLOWED`, the path is not in this iteration's files.

- [x] Give `NewSession` the iteration's file list alongside the root. An empty
      list means the whole root, so existing callers keep their behaviour
- [x] Refuse a read or an edit outside the list, in the wording the
      architecture's error table gives
- [x] Return that refusal as the tool's result, so it costs one turn and not the
      run
- [x] Break it: a session given one file must refuse a read of its sibling, and
      the same session with an empty list must serve both

> **Completed** 2026-08-25 15:13 UTC
>
> - `make check` — 1.4s

## Iteration 7: remove every reference to a private network

This repository will be public. It names a machine on a private network in four
places and carries two addresses from a private tailnet.

| Where                                     | What                             |
| ----------------------------------------- | -------------------------------- |
| `cmd/ratchet-dev/drive.go`                | `--host` defaults to a host name |
| `internal/agent/ollama_test.go`           | that host name, twice            |
| `internal/agent/ollama_test.go`           | a tailnet IPv6 prefix            |
| `docs/product.md`, `docs/architecture.md` | a second host name, three times  |
| `docs/architecture.md`                    | a tailnet IPv4 address           |

`--host` loses its default. A default naming one machine is wrong in a public
repository, and this particular name resolves to an interface measured as the
one that drops. Anyone wanting a default has an environment variable or a config
file.

Replacements come from the ranges reserved for documentation, so nothing
resolves and no reader mistakes an example for a real host: RFC 5737 for IPv4,
RFC 3849 for IPv6, and a generic word for a host name.

- [x] Make `--host` required with no default, and say in its usage where a
      person keeps one
- [x] Replace every host name and address in code, tests and docs, keeping each
      test's meaning: the IPv6 case still needs a bare address with no port to
      exercise the bracketing
- [x] Add a check under `internal/convention` that fails on a private or tailnet
      address, or on either host name, anywhere in the repository
- [x] Break it: reintroduce one address and watch the check fail

> **Completed** 2026-08-26 03:23 UTC
>
> - `make check` — 1.4s

## Iteration 8: make the spec agree with the code

- [x] Correct the departure note that says the context check is unfixed. The
      same change set fixes it both ways the note proposed, and a reader trusts
      the spec over a comment
- [x] Change `runDrive` to take an `io.Writer` rather than an `*os.File`,
      matching every sibling command, so its output can be captured
- [x] Add a test that drives `runDrive` against the scripted provider and
      asserts the transcript names each turn
- [x] Re-read the spec against the code and record anywhere else they disagree

> **Completed** 2026-08-26 03:27 UTC
>
> - `make check` — 1.4s

## Where this plan was departed from

**The decision to check `Allocated` before the first turn is reversed.** A host
lists what it gave a model only once it holds one, so the check written to
refuse too little context refused every model that had not already been used
instead, and the live run below needed one warmed by hand. The check now runs
after the first turn, which is the request that loads the model, and
`ErrNotLoaded` separates a host that is not holding the model from a host that
gave it too little. `docs/architecture.md` said the read happens before starting
and now says when it happens and why.

**The live run passed on the first attempt, and exercised only the happy path.**
Three turns: read, one `SUB` patch citing the tag from that read, `done` with an
accurate summary. The file on disk holds the change. No refusal, no protocol
error and no retry occurred, so the parts built to handle a model's mistakes are
still untested against a real model.

**The model chose a longer fragment than the prompt taught.** The prompt's `SUB`
example replaces a bare identifier; the model sent `-function midpoint` rather
than `-midpoint`. That is the safer choice and the prompt did not ask for it.

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

**The Provider sketch in the design named a shape the code never had.**
`docs/architecture.md` gave `Stream` a channel of events and an error; it takes
a callback and returns the reply. Corrected in the doc, because a design doc
describes the target and this described one abandoned before the code existed.
