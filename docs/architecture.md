# Ratchet: architecture

> Written as if the implementation shipped. None of it exists yet. The product
> it describes is [product.md](product.md)

## What runs where

Five things run. Getting the boundaries right settles most of the rest.

```
  your shell ──── ratchet (one binary, on the host)
                    │
                    ├── owns: git, the specs, the journal, the index
                    ├── runs: the agent loop, the gate runner, the drafting server
                    │
                    ├──► model host        HTTP, over the network
                    │      ollama / mlx / vllm, here or on another machine
                    │
                    └──► sandbox           one container per run
                           full repo toolchain, no git, no agent
                           docker or podman; capabilities differ
```

**The agent loop runs on the host, not in the sandbox.** The loop talks to the
model, parses its output, classifies failures and decides whether to retry. None
of that needs sandboxing, and putting it inside means the sandboxed process is
also the process reporting whether the sandbox behaved.

The model never touches anything directly. It emits tool calls and Ratchet
implements them, so containment is a question about each implementation, and the
answer is that all but one are Go functions on the host. The exception is
`bash`.

That makes the container a **shell sandbox**, not an agent sandbox. It says
nothing about how much software the image holds. The image is not minimal: it
carries the repo's whole toolchain, because `bash` runs `npm test` and a gate
that cannot execute is not a gate.

```dockerfile
FROM node:22-bookworm-slim          # pinned; the repo's runtime
RUN apt-get install -y --no-install-recommends \
      build-essential python3 ca-certificates   # native modules need a compiler
RUN apt-get purge -y git && rm -f /usr/bin/git  # absence, not policy
ENV PAGER=cat GIT_PAGER=cat NO_COLOR=1 CI=1 \
    NPM_CONFIG_PROGRESS=false NPM_CONFIG_FUND=false NPM_CONFIG_AUDIT=false
# No USER line: the entrypoint needs root to install the egress allowlist,
# then drops to uid 1000, which the explicit uidmap makes the host user.
ENTRYPOINT ["/usr/local/bin/ratchet-entry"]
```

**There is no `pi` in the image because there is no `pi` in this design.** In
the research setup that produced these measurements `pi` was the harness, so
containerizing it would have put the harness inside the sandbox. Ratchet
replaces `pi`, which leaves only command execution to contain.

`read`, `edit` and `write` work from the host against the inodes the container
has bind-mounted. Routing them through `docker exec` would cost a process spawn
per call and buy nothing.

Splitting file writes from command execution has one wrinkle: ownership.
`npm install` writes `node_modules` as the container's uid and the host then
edits those files, so the container runs with the invoking user's uid preserved
and `ratchet doctor` checks it can write to the mount before a run rather than
failing on iteration 1. How the uid is preserved differs between docker and
rootless podman, which gets its own section.

Two consequences fall out free. `git` lives on the host and nowhere else, so the
executor has no path to `checkout` without a policy. And gates run in the
container too, so `npm test` passing under a gate means what it meant when the
executor ran it.

The model is never in the container. It is a network service. Ratchet does not
manage its lifetime beyond a warmup call and a `keep_alive`, because a 20GB
model that reloads between iterations costs ~55 seconds each time.

## The binary

Go. One statically linked binary, cross-compiled for darwin/arm64 and
linux/amd64.

The requirements pick the language. Ratchet supervises subprocesses with
timeouts and clean kills, serves HTTP with server-sent events, speaks to model
endpoints, drives containers, and starts fast enough that `ratchet list` feels
instant. Go does all of that in the standard library, and single-binary
distribution matters more than it sounds: this tool gets copied to a laptop, a
GPU box and a CI runner, every one of which is a place a Python environment
would rot.

The binary embeds the drafting page: Go templates, one stylesheet, a vendored
htmx, and the TypeScript it compiles at startup. `go build` and `go install`
need Go and nothing else.

A `Makefile` wraps the usual targets. `go build ./...`, `go vet ./...` and
`go test -race ./...` pass before anything is committed. `make web-check`
typechecks the client sources; there is no asset build target because there is
no asset build.

### Packages

Everything is `internal/`, and there is no root package: `main.go` imports the
front ends directly.

```
internal/agent            the loop: providers, streaming, dispatch, classification
internal/anchor           hash-anchored line addressing
internal/api              the wire schema: --json result types and their mappers
internal/cli              the Run* actions, one file per command noun
internal/convention       tests that hold this repo to its own conventions
internal/drafter          the drafter seat: the five passes, its prompt, revision
internal/drafter/claude   the subscription path: drives the claude CLI
internal/drafter/session  the collaboration surface: HTTP, SSE, threads, mockups
internal/drafter/tool     read, grep, index, bash, write, edit, ask, choose,
                          decide, mockup; also over MCP for a loop we do not own
internal/edit             resolve an anchor, apply a patch in memory
internal/executor         the executor seat: its prompt, its budgets
internal/executor/tool    read, edit, write, bash, revert_file, done, blocked
internal/fixture          distill harness journals into replay fixtures
internal/gate             gate execution, the mutation sweep, worktrees
internal/index            the repo index and its language providers
internal/journal          append-only event log; also the replay source
internal/notify           the notification channels
internal/patch            parse the two measured edit forms
internal/sandbox          engines, container lifecycle, exec, egress rules
internal/spec             parse and render specs: the ratchet blocks, the state
                          block, the fold that produces status
internal/testutil         shared fixtures, stdlib-only so any test can import it
main.go                   the only package main; the urfave/cli v3 command tree
```

**Nothing is public until something needs it.** A package outside `internal/` is
a compatibility promise, and promising compatibility on a design still moving is
how you end up maintaining a shape you already regret. `spec`, `anchor` and
`patch` would go public first, and are written so the move stays cheap: no
globals, no dependency on anything else in the module, no assumption that a spec
came from a file. `edit` is one step behind them, depending on `anchor` and
`patch` and on nothing else.

**The two seats are packages and `agent` is not one of them.** Both are
model-driven loops with providers, streaming, dispatch, classification and
retry, so that machinery is one package used by both. What differs is the prompt
and the tool set, which is what `drafter` and `executor` own. Naming the loop
`executor` would have been tidier and wrong: it would imply the drafter does not
use it, and the drafter is the heavier user.

Each seat's tools nest under it because the sets are genuinely different and
neither is general. `drafter/session` nests for the same reason: that server
exists only to mediate between the human and the drafter, so it is not a web
layer that happens to serve drafting. Its central type is `session.Session`,
which is what `ratchet plan` opens.

```
main.go → cli → {drafter, executor, gate, index, journal, notify} → {spec, anchor}
       drafter  → {agent, drafter/claude, drafter/session, drafter/tool}
       executor → {agent, executor/tool, sandbox}
       executor/tool → edit → {anchor, patch}
       cli → api → {spec, journal}
```

`drafter` and `executor` are independent siblings. `spec` has no dependency on
`agent`, because parsing a spec must be possible without the ability to run one:
`ratchet list` and `ratchet verify` run where no model exists.

`internal/api` holds the `--json` shapes and sits below `cli`, so the CLI and
the server emit one contract by construction rather than by discipline. The wire
types are not the parse structs; states and ids are strings out there, because
that surface is a promise and the internal model is free to change.

### Conventions

Moved to [conventions.md](conventions.md), which is where a reader looks for how
code is written rather than how the system is shaped.

## Storage

No database. Four kinds of persistent state, each in the cheapest thing that
holds it.

| What               | Where                                   | Committed | Format     |
| ------------------ | --------------------------------------- | --------- | ---------- |
| specs, incl. state | wherever your specs live                | yes       | markdown   |
| preferences        | `$XDG_CONFIG_HOME/ratchet/config.json`  | no        | JSON       |
| credentials        | `$XDG_DATA_HOME/ratchet/auth/`, 0700    | no        | per-vendor |
| index cache        | `$XDG_CACHE_HOME/ratchet/index/<repo>/` | no        | gob        |
| a live server      | `$XDG_RUNTIME_DIR/ratchet/<spec>.json`  | no        | JSON       |
| repo settings      | `<repo>/.ratchet/config.json`           | **yes**   | JSON       |
| run journal        | `<repo>/.ratchet/runs/<spec>/`          | no        | JSON lines |
| session threads    | the journal                             | no        | JSON lines |

User-global state goes to XDG, per-working-tree state stays in the tree. That
line settles every case, and the XDG halves land in four different homes, which
suggests the spec's distinctions are real rather than pedantry.

Unset, they expand to these. Only three of the four have a specified default.

| Variable          | Default          | Ratchet's path when unset           |
| ----------------- | ---------------- | ----------------------------------- |
| `XDG_CONFIG_HOME` | `~/.config`      | `~/.config/ratchet/config.json`     |
| `XDG_DATA_HOME`   | `~/.local/share` | `~/.local/share/ratchet/auth/`      |
| `XDG_CACHE_HOME`  | `~/.cache`       | `~/.cache/ratchet/index/<repo>/`    |
| `XDG_RUNTIME_DIR` | **none**         | `$TMPDIR/ratchet-<uid>/<spec>.json` |

`XDG_RUNTIME_DIR` has no default in the specification, which tells applications
to fall back to a directory with similar properties and warn. On Linux under
systemd it is `/run/user/<uid>`; on macOS it is usually unset, and `$TMPDIR` is
already per-user and mode `0700`, so that is the fallback, created `0700` under
a uid-qualified name.

That fallback breaks a claim made above. A real runtime directory is swept at
logout, so a file's existence answers "is a server up?". `$TMPDIR` is not
reliably swept, so the file cannot be trusted for that. It records the pid and
the port, and a reader checks the pid is alive and is Ratchet before believing
the port. Existence is not liveness anywhere it matters, and a stale pidfile
that reads as a running server is the same silent-failure shape as everything
else here.

`XDG_STATE_HOME` (`~/.local/state`) is unused. The journal would be its
candidate and the journal is repo-local, so nothing lands there.

Preferences are hand-edited, so `CONFIG_HOME`. Credentials must survive and are
not editable, so `DATA_HOME` at mode `0700`. The index is derived and deletable
at any moment, so `CACHE_HOME`, which also gets a `<treehash>` blob out of the
repo and out of the gitignore. A running server's address is ephemeral and tied
to a login session, so `RUNTIME_DIR`, and being swept on logout is the correct
answer to "is a server still up?"

The journal stays in the repo, and that is not an XDG exception. The journal is
to a repo what `node_modules` or `target/` is: derived state about one working
tree, whose snapshots are diffs against that tree and cannot be read anywhere
else. Divorcing them means a clone on another machine reads someone else's
history, and a deleted repo leaves gigabytes orphaned under `~/.local/state`. It
is also the difference between `tail -f .ratchet/runs/002/journal.jsonl` while
sitting in the repo and going to find where the tool decided to put it.

The repository's `.ratchet/` is partly tracked: `config.json` and a `.gitignore`
naming `runs/` are committed, and everything else in there is derived.

There is no `~/.ratchet`. A tool that invents its own dotfile directory next to
four standard ones is asking every user to learn a fifth convention.

On macOS Ratchet uses the XDG paths too rather than
`~/Library/Application Support`. Apple's answer is right for applications and
wrong for a command-line tool whose users expect to find their configuration by
grepping `~/.config`. The environment variables are honored on every platform,
which is also how the tests relocate all of this into a `t.TempDir()`.

SQLite loses on the access patterns. The journal is written append-only and read
sequentially, which is a log file. The index is written once per tree hash and
read whole, which is a serialized blob. Neither wants indexes or a query
planner, and a database would put the audit trail in a format you cannot
`tail -f` during an unattended run.

### Preferences

Four settings are properties of a machine rather than a repository: which
container engine, which executor, where to bind the page, how to notify you.
Each differs between a laptop and a GPU box while the spec stays the same.

```json
{
  "container_engine": "podman",
  "executor": "glm-4.7-flash@block",
  "listen": "100.92.194.77:7000",
  "notify": { "kind": "telegram", "chat": "…" }
}
```

Precedence is per key, not per file: `--container-engine` overrides that setting
and leaves the rest alone. Flag, environment, this file, then a default or
detection.

The file exists because environment variables are awkward to set durably. They
live in a shell profile, a launchd plist, a systemd unit, or nowhere at all when
cron runs the command, and a value that works interactively and vanishes under
automation is a bad place to keep the setting that decides which machine your
model runs on.

**An unknown key is an error, not a shrug.** A config that silently ignores what
you wrote is the same failure as a gate that cannot fail. `doctor` reports the
file it loaded and the keys it took, and Ratchet refuses to start on a key it
does not recognize.

It holds preferences and never credentials. API keys come from the environment
or the provider's own store, and the subscription credential lives in its own
directory at mode `0600`.

This file is one of three scopes, and the test for which is whether two things
that share the scope could legitimately differ.

| Scope   | Where                                  | Committed | Holds                         |
| ------- | -------------------------------------- | --------- | ----------------------------- |
| machine | `$XDG_CONFIG_HOME/ratchet/config.json` | no        | what this machine provides    |
| repo    | `<repo>/.ratchet/config.json`          | **yes**   | what this codebase needs      |
| work    | the spec's header `ratchet` block      | yes       | what this piece of work needs |

Same basename at two scopes, same format, narrower wins. The paths disambiguate,
and the parallel is deliberate.

```
.ratchet/
  .gitignore        tracked. contains: runs/
  config.json       tracked. repo-scoped settings
  runs/<spec>/      ignored. journal, snapshots, lock
```

Ratchet ships its own `.gitignore` inside its own directory, so adopting it
never edits the repo's root ignore file. That file has its own history and
conventions and merge conflicts, and a tool that appends to it is a tool you
notice again on every rebase. `.vscode/` and `.idea/` solve it the same way.

Mixing tracked and ignored content in one directory has one hazard, and it is a
loud one: `rm -rf .ratchet` to clear a cache also deletes two tracked files,
which `git status` reports as deletions rather than swallowing.

```json
{
  "worktree_setup": "npm ci",
  "image": "ratchet-exec:node22",
  "write_root": ["specs/", "docs/"],
  "egress": ["registry.npmjs.org"]
}
```

Narrower wins: a spec header overrides the repo config, which overrides a
default or something derived.

**The repo config is optional, because the index derives most of it.**
`Commands` on an index provider returns setup, build and test, read from
`package-lock.json` or `go.mod` or `uv.lock`, which covers the ordinary case
with no file at all. The file exists for the cases derivation gets wrong, and
for the two things it cannot reach: which hosts the gates need to talk to, and
where the drafter may write.

**An earlier version of this document said there was no repository-level config,
and that was wrong.** The argument was that repo-scoped facts already live in
the spec, citing `executor_class` and `min_executor_context_window`. Those two
are properties of _the work_, not of _the codebase_: they record how the spec
was sized. A setup command, a toolchain image, an egress allowlist and a write
root are properties of the codebase, and putting them in a spec header means
four specs in one repo carry four copies that can drift.

The scope test settles each one. Could two specs in the same repo legitimately
need a different `npm ci`? Almost never, so it is repo-scoped. A different
context window, because one was sized for a 32k executor and another for 48k?
Yes, so that is work-scoped. A different container image, in a monorepo with a
node half and a python half? Occasionally, which is why the spec can override.

### The journal is the audit trail and the test fixture

One file per run, one JSON object per line, append-only, fsynced at iteration
boundaries.

```json
{"t":"…","seq":41,"kind":"model_request","iter":"i-9e2b14","attempt":2,"tokens_in":18422}
{"t":"…","seq":42,"kind":"model_response","stop":"tool_call","tokens_out":214,"ms":5310}
{"t":"…","seq":43,"kind":"tool_call","name":"edit","args":{"path":"src/game.js","anchor":"402#SN"}}
{"t":"…","seq":44,"kind":"tool_result","ok":true,"lines_changed":17}
{"t":"…","seq":45,"kind":"snapshot","diff_sha":"a91c…","bytes":2841}
```

Every event carries a monotonic `seq`, so a truncated final line is detectable
and a partial write is recoverable by discarding it. Nothing reads the journal
while it is being written; we produced two false results by scoring a log
mid-write, so the reader takes the lock the writer holds.

The useful part is that a journal replays. `model_request` and `model_response`
pairs are a complete record of what the model saw and said, so a recorded run
becomes a deterministic test.

## The agent loop

```
  for attempt := range attempts {
      ctx := prompt(iteration, attempt)
      for {
          resp := provider.Stream(ctx)        // idle timer resets per token
          switch classify(resp) {
          case Transport:   retry without consuming an attempt
          case Protocol:    repair or shrink; consume an attempt
          case Budget:      escalate; do not re-issue the same request
          case Ok:          break
          }
      }
      results := dispatch(resp.ToolCalls)     // one call per turn
      ctx = append(ctx, resp, results)
      if terminal(resp) || over(budget) { break }
      snapshot()
      if repetitive(history) { halt }
      if idle(lastEvent) > allowance { abandon }
  }
  gate.Run(iteration)                          // the runner decides, not the model
```

### Providers

`internal/agent` is written against one seat-agnostic interface, so the drafter
and the executor differ only in which provider they are handed and which tools
they carry.

```go
type Provider interface {
    Stream(context.Context, Request) (<-chan Event, error)
    Allocated(context.Context, string) (contextWindow int, err error)
}
```

`Allocated` exists because of a measured lie. Ollama's OpenAI-compatible
`/v1/chat/completions` silently discards `options.num_ctx`, so a model asked for
48k loads at the server default of 4096 and nothing reports it. Ratchet speaks
Ollama's native `/api/chat` and reads the real allocation from `/api/ps` before
starting. `min_executor_context_window` is checked against that number, not the
one we asked for.

Implementations: `ollama`, `openai` for anything OpenAI-shaped including
mlx-openai-server and vLLM, and `anthropic`. They are compiled in rather than
plugged in. There are four, a few hundred lines each, and a plugin boundary
would buy extensibility nobody asked for at the cost of a stable ABI over a
surface we are still learning.

### The drafter is driven at a coarser grain than the executor

A subscription is not an API key, and that changes the architecture rather than
adding a provider.

Anthropic subscription access is reachable through the `claude` CLI, which
authenticates with a browser OAuth flow and bills nothing per token. But
`claude` is not a chat-completions endpoint. It is an agent with its own loop,
tools and permission system, and driving it through `Provider` would put a loop
inside a loop with Ratchet's dispatch, classification and budgets bypassed by
the inner one.

So the two seats get different interfaces, and the transport is not the reason.

The executor needs Ratchet to own its loop. Every guarantee here lives there:
operation budgets, idle detection, repetition halting, classification, edit
validation, snapshot-per-operation.

The drafter does not. Its output is a markdown file, its phase has a human in
it, there is nothing to snapshot and no budget beyond the patience of whoever is
watching. Ratchet needs a spec and a stream of questions, and does not care who
ran the loop.

```go
type Drafter interface {
    Draft(context.Context, DraftRequest) (<-chan DraftEvent, error)
}
```

`agent.Drafter` runs Ratchet's loop for API keys and local models.
`claude.Drafter` shells out:

```
claude -p --output-format stream-json --permission-mode acceptEdits \
       --mcp-config <generated> \
       --append-system-prompt <the drafter instructions>
```

**The drafter's tools do not change when the loop does.**
`internal/drafter/tool` gets an MCP front end, so all ten are one set of
implementations reached two ways: as Go functions, or over stdio. Same
semantics, same journal events, one place to change them. The executor's tools
are a separate set and stay separate; what is shared is a transport, not a tool
set.

Without that the two drafter paths would drift, and the subscription one would
quietly lose the distinction between a recorded decision and a blocking
question.

The subscription path gets the same policy, not an exemption. `claude` has its
own Bash, Edit and Write, so nothing Ratchet declares in a schema restricts it.
Containment comes from the mount: it runs inside the drafter's container with
the repo read-only, its config directory writable, and `--tools ""` so it is
told rather than left to discover. `--tools` is the flag that restricts the
built-in set; `--strict-mcp-config` restricts MCP servers only, and
`--allowedTools` is a permission list, not a tool set. Measured under
`--tools ""`, the session's whole surface is Ratchet's MCP tools, because MCP
tools are additive and `--tools` never removes them. The flag is the courtesy;
the mount is the enforcement. Two drafters with different blast radii can do
different things, and then a difference in their specs is a difference in what
they were allowed.

Unattended launch turned out to be the easy part. Under `-p` there is no
first-run prompt to hang on: an empty config directory fails immediately with
`error: "authentication_failed"`, and the workspace trust dialog is documented
as skipped in non-interactive mode. Seeding is one file — `.credentials.json` at
mode `0600`, 509 bytes, no keyring involved — and `claude auth status` returns
JSON, so `doctor` asks the binary rather than parsing the config itself.

Two flags to avoid, both of which look like the isolation answer. `--safe-mode`
drops MCP servers entirely, including one named on an explicit `--mcp-config`.
`--bare` reads Anthropic auth only from `ANTHROPIC_API_KEY` or `apiKeyHelper`,
so it cannot use the subscription at all.

The failure to classify carefully is `result.subtype`, which reports `success`
on a run whose `is_error` is `true` and whose `terminal_reason` is `api_error`.
Classification reads `is_error` and `terminal_reason`; a classifier keyed on
`subtype` records an auth failure as a completed turn.

`CLAUDE_CONFIG_DIR` is pointed at `$XDG_DATA_HOME/ratchet/auth/claude/`, so the
credential lands in Ratchet's own data directory at mode `0600` rather than in
the user's `~/.claude`, and is never copied into a repo or an image.

Three costs. There is **no token accounting**, so the drafter is budgeted in
wall clock and turns. The **provider is a process, not a request**, so
classification is coarser: `transport` and `capability` stay distinguishable,
`protocol` and `budget` mostly do not. And **rate limits belong to the
subscription**, so a long session can be throttled by something Ratchet cannot
see. It surfaces the wait rather than retrying into it.

### Streaming is mandatory

Not for latency. For the idle timer.

Budgets are operations and idleness, and idleness is time since the last event.
Without streaming there are no events between request and response, so a hung
generation and a slow one are indistinguishable. A passing attempt in our runs
went silent for 425 seconds while doing real work, and a total timeout
calibrated to kill hangs killed it.

Streaming also gives early truncation detection. Ratchet tracks brace depth and
in-string state per tool-call index as tokens arrive, and if either is non-zero
at stream end the response was truncated regardless of what `finish_reason`
claims. Local servers report finish reasons wrongly often enough that the
parser's own opinion is the more reliable one.

### Tool calls

Prefer the server's structured tool calls. Fall back to a parser.

Ollama's per-family parsers break when argument content contains `<`,
`</function>` or `<parameter>`, which is common in code, and the failure mode is
that the call arrives as prose with no error. So the Ollama provider requests
raw text for families with a known-broken parser and runs
`internal/agent/toolparse` itself. Which families is a table, not a heuristic,
and the table cites upstream issues.

Repair follows Cline's rule: run a JSON repair pass only when there is no
unterminated string, because closing one produces a valid-looking wrong value.
Truncation is never repaired. It is `Protocol`, and the retry gets a smaller
context.

**A malformed call becomes a tool result carrying the error**, never a dropped
turn. The model sees its own mistake next turn, and that single behavior is the
largest correctable gap in a naive harness.

### Failure classification

```go
func classify(r Response, err error) Class {
    switch {
    case isNetwork(err), isServer5xx(err):        return Transport
    case r.Truncated, r.Unparseable:              return Protocol
    case r.ContextExhausted, r.OverOperations:    return Budget
    case r.Empty && r.Stop == "length":           return Budget
    default:                                      return Capability
    }
}
```

The fourth case is a scar. Setting `supportsFinishReason: false` on a provider
turned an empty generation from a loud retryable failure into a silent success,
and an iteration that had been passing on attempt two started failing after
three. An empty response with `stop: length` is context exhaustion. It is never
a capability failure and it must never be silent.

### Repetition, not volume

The loop guard keys on `(tool, normalized args)`, with a warn state before a
halt state and a counter that resets when a different mistake occurs.

Thresholds come from measurement. Passing iterations reached ten consecutive
calls to the same tool; the pathological loop reached 183. Qwen Code's default
of eight would have killed every passing run we have, because with seven tools
the tool identity carries almost no signal and the argument shape carries it
all. `bash("npm test")` twice is fine. `bash("echo ITERATION 5 COMPLETE")` 183
times is not.

## Tools

All synchronous, one call per turn. The turn limit is structural: no schema has
a batch form, so a partially applied multi-edit is not a state the system can
represent.

The two seats have disjoint tool sets, which is why they are separate packages.

Each tool advertises a strict JSON Schema and validates against a permissive
union. This is Cline's split. The strict schema is what the model is shown; the
union is what `execute()` accepts: `path` or `paths` or `file_path`, a bare
string where an array is declared, `"3"` where a number is. Advertising the
permissive one would teach sloppiness; accepting only the strict one discards
calls whose intent is unambiguous.

### The executor's seven

```
read(path)                        → line, "#", hash, ":", text  per line
edit(path, anchor, end?, text)    → applied
write(path, text)                 → applied
bash(cmd)                         → {output: capped 10k head+tail, exit: int}
revert_file(path)                 → restored to the iteration-start snapshot
done(summary)                     → terminal. a claim, not a decision
blocked(reason)                   → terminal. stops the iteration
```

| Error                | Raised by                 | Means                                         |
| -------------------- | ------------------------- | --------------------------------------------- |
| `E_FILE_NOT_ALLOWED` | read, edit, write, revert | the path is not in this iteration's `files`   |
| `E_FILE_NOT_FOUND`   | read, edit, revert        | it is allowed and does not exist              |
| `E_ANCHOR_MISMATCH`  | edit                      | the hash does not match; see the two branches |
| `E_EDIT_REJECTED`    | edit                      | the result failed validation; nothing applied |

Codes read `E_<subject>_<condition>`, so the subject comes first, and
`NOT_ALLOWED` says what happened rather than naming the field it happened to.

`done` and `blocked` are the only terminal verbs and both are named in every
failure message the executor sees. There is no verb for closing an iteration,
satisfying a gate, or supplying an ack.

### The drafter's ten

```
read(path)                        → hash-anchored lines, as the executor sees
grep(pattern, glob?)              → matches with paths and line numbers
index(query?)                     → the repo snapshot, or one answer from it
bash(cmd)                         → {output: capped, exit: int}
write(path, content)              → applied
edit(path, anchor, end?, text)    → applied
ask(question, iteration?)         → the answer text, or ErrUnanswered
choose(options[], iteration?)     → Choice, or ErrUnanswered
decide(statement, from?)          → recorded. never waits
mockup(name, variants[])          → Choice over the variants, or ErrUnanswered
```

| Error                    | Raised by                   | Means                                         |
| ------------------------ | --------------------------- | --------------------------------------------- |
| `E_FILE_NOT_ALLOWED`     | read, grep                  | outside the repo                              |
| `E_FILE_NOT_FOUND`       | read, edit                  | inside the repo and absent                    |
| `E_PATH_NOT_WRITABLE`    | write, edit                 | readable, but outside the write root          |
| `E_STATE_BLOCK_READONLY` | write, edit                 | at or below the `ratchet:state` marker        |
| `E_ANCHOR_MISMATCH`      | edit                        | the hash does not match; see the two branches |
| `E_EDIT_REJECTED`        | write, edit                 | the spec would no longer parse; not applied   |
| `E_BAD_PATTERN`          | grep                        | the expression does not compile               |
| `E_UNKNOWN_QUERY`        | index                       | not a query it answers; the error lists which |
| `E_TOO_FEW_OPTIONS`      | choose, mockup              | fewer than two were supplied                  |
| `E_NO_SUCH_ITERATION`    | ask, choose, decide, mockup | the `iteration` id does not resolve           |
| `E_NO_SUCH_THREAD`       | decide                      | `from` does not resolve                       |
| `ErrUnanswered`          | ask, choose, mockup         | the deadline lapsed. expected, not a fault    |

The first four are the executor's codes, declared once in `internal/agent`,
which both seats import. A path error must not mean one thing here and another
there.

`E_EDIT_REJECTED` is narrower for the drafter. Markdown has no compiler, so the
validation is the spec parser: an edit that would leave a `ratchet` block
unparseable is refused rather than written. The drafter cannot hand the runner a
spec it cannot read.

`E_TOO_FEW_OPTIONS` guards a choice of one, which is a decision wearing a
consultation. `E_NO_SUCH_ITERATION` fails loudly because a question attached to
nothing renders nowhere, and the drafter would wait on an answer you were never
shown.

`bash` has no tool errors in either seat. A non-zero exit is a result the model
reads, and an unreachable container is a transport failure the loop classifies.

### Three outcomes, two of which are values

`ask`, `choose` and `decide` are separate tools because the difference between
them is the thing worth getting right. `ask` waits. `choose` waits with the
options enumerated, so the work of listing them is not handed back to you.
`decide` records an answer and keeps going.

`choose` can end three ways: you pick, you reject all of them and say what you
want instead, or you let it lapse. As a tagged union that is bad Go: three
pointer fields, or an interface and a type switch, and states that can be built
invalid.

Sorting by kind fixes it. **Lapsing is not an outcome, it is the absence of
one**, which in Go is an error.

```go
var ErrUnanswered = errors.New("no answer before the deadline")

func (t *Tool) Ask(ctx context.Context, req AskReq)       (string, error)
func (t *Tool) Choose(ctx context.Context, req ChoiceReq) (Choice, error)
func (t *Tool) Mockup(ctx context.Context, req MockupReq) (Choice, error)
```

One sentinel for all three waiting tools, tested with `errors.Is`. A lapsed
deadline takes the same path as a dropped connection, which is what it has in
common with one.

The other two are not a union either. Both answer what was decided, and either
way the drafter's next move is `decide` with a statement.

```go
type Choice struct {
    OptionID string // "" when the human supplied their own
    Text     string // the chosen option's text, or the human's
}

func (c Choice) FromOptions() bool { return c.OptionID != "" }
```

No discriminant, no invalid state to guard, `Text` always carrying what the
drafter consumes. It also removed a type: `mockup` returns the same `Choice`,
because a mockup is a choice whose options are documents.

The alternative has to exist however it is represented. Without it a `choose` is
a leading question, since the drafter enumerated three architectures and is not
the party with the final say about whether those are the three. It is also
information: a choice answered in free text means the enumeration was wrong, and
that rate measures how well the drafter understands the codebase, the way
`BLOCKED` measures how well the executor was specified.

### decide, not assume

A decision reaches a spec four ways: the drafter picked a defensible default,
you answered an `ask`, you picked in a `choose`, or you said something in a
thread that settled it. All four end as a statement written into the iteration
that depends on it, so one verb records all four.

```
decide("node --test, not vitest; no new dependency", from: "th-4a91c2")
decide("overlaps() stays in entities.js")                 // the drafter's own call
```

`from` names the thread or choice it came from, absent when the drafter decided
alone. So every decision traces to a row saying who settled it. `assume` could
only have covered the first of the four.

### What "blocks" means

`ask`, `choose` and `mockup` wait for a human, which is only tolerable because
of where they live. The same call in the executor would be a hang, which is why
the executor has `blocked` and gets its answer as a revised spec.

**Every wait has a deadline and the deadline is part of the contract.** The call
returns an answer or `ErrUnanswered`.

```
ask       15 min    typing "node" does not take longer than that
choose    30 min    reading three options with costs does
mockup     2 h      clicking through layouts at different widths does
```

A notification fires the instant the call is made, so the wait starts when you
learn about it rather than when the model gives up.

`ErrUnanswered` has a documented response: record a decision and continue, or
stop and list what is outstanding. Which is the behavior already specified for a
decision with a defensible default, so the timeout branch collapses into a rule
that exists.

How the wait is implemented differs; the contract does not. On Ratchet's own
loop we own the message list, so a long wait suspends the session to the journal
and exits rather than holding a process for two hours, and answering resumes it
with the tool result appended. On the `claude` path the MCP call blocks, because
a tool call is synchronous in that protocol.

That path was the highest risk in this document and it holds: **a 25-minute
blocking MCP call returns cleanly**, `terminal_reason: "completed"`, with the
answer intact. Better, the harness heartbeats the pending call every 30 seconds
— 49 events, no gaps, over the whole wait — so the progress display and the
liveness check are the same signal, and a call that dies is distinguishable from
a human who is slow without a timeout of Ratchet's own. `ask`'s 15-minute
deadline sits comfortably inside what was measured; `mockup`'s two hours does
not, and is the one deadline still resting on an assumption.

The `claude` path also supports `--input-format stream-json`, which keeps one
process across many turns under a single session id, so a drafting conversation
does not pay to reload a transcript per message. An `init` event arrives per
turn with the session id unchanged; reading `init` as "new session" would split
one conversation into several.

`decide` never waits, which is what makes the deadlines survivable. Used as
instructed, blocking calls are rare, and three in one session says something
about the spec rather than the tool. That is also why `ask` takes one question
rather than a batch: batching is the right shape only if blocking is common, and
if blocking is common the problem is upstream.

### write is scoped, edit is anchored

Drafting produces more than one document: a plan comes with the notes that
justify it, a diagram, a findings file. A drafter that can only write the spec
loses that work or smuggles it in.

So it has a **write root**, a declared set of paths it may write anywhere under,
defaulting to where the specs live and widened in the repo config to something
like `docs/`. Declared rather than inferred, because "everything except source"
is not enforceable. It is mounted writable in the container too, so a script can
emit an artifact, and everything it produces is a file under a path you
nominated, which makes reviewing the drafting phase `git status`.

`edit` exists because revision is the common case. Comments come back and the
drafter changes iteration 6, leaving eleven alone. With only `write` that means
re-emitting the whole document, at six to eight thousand output tokens, and
putting every untouched iteration back through a model that was not asked to
change it. A reflowed sentence in iteration 9 is a change nobody reviews,
because it arrives inside a diff too large to read.

Anchors also cover a case specs have and code does not: you may be editing the
spec in your editor while the drafter revises it, because it is a file in your
repo and that is the point. A mismatched anchor detects that and returns the
real one.

`sed` through the shell is possible and is the wrong path, since that write
carries no journal event, no validation and no snapshot. Ratchet diffs the write
root at every turn boundary and journals what changed outside a tool call as
`external_write`. Detected and attributed, not prevented by a rule nobody can
enforce.

### The drafter has a shell

Its job is to understand a codebase well enough to cut seams and size
iterations, and you do not learn what a build does by reading it.

Withholding one would also break the only comparison worth making. The open
question about tier 3 is whether a large local model can draft as well as a
hosted one, and a hosted one runs in its own harness with a shell. A local
drafter without one loses on the harness rather than on the model, which is the
confound this project keeps finding.

So both seats have a shell, both are contained, and the policy differs.

|             | Executor                     | Drafter                      |
| ----------- | ---------------------------- | ---------------------------- |
| repo mount  | read-write                   | **read-only**                |
| write scope | the iteration's `files` list | the declared write root only |
| `git`       | absent                       | absent                       |
| egress      | registry, model host         | registry, model host         |

The drafter can run and read anything and cannot modify source, because its
write verbs are host-side tools. Same split as the executor one level over.

### Hash anchors

`internal/anchor` gets its own package because two things must agree: the
renderer that shows a model a file, and the resolver that interprets an address
afterwards.

**One tag per file, not one per line.** A read is stamped with a hash of the
whole file and its lines carry bare numbers.

```go
// Tag is the four-hex fingerprint carried by a rendered read. Trailing
// whitespace is stripped per line first, so a display-trimmed or CRLF copy of
// the same content mints the same tag.
func Tag(text string) string {
    return fmt.Sprintf("%04X", xxhash32(normalize(text))&0xFFFF)
}
```

The alternative — a hash on every line, keyed on the line and its neighbours —
was measured and rejected. It is not free: it cost the weaker two of four models
15 and 19 correct answers in 30, against a file tag that cost nothing at all
(83/90 either way). The reason is arithmetic rather than anything subtle. A
per-line scheme asks the model to transcribe an opaque token once per address; a
file tag asks once per file, and the models get that right essentially always.

The property the per-line version buys is real and tiny: it can catch a
line-number slip when the model copies the intended line's hash, which a file
tag cannot. Measured, that fired once in 120 attempts, against 41 refusals the
file tag never incurs.

Resolution is exact: recompute the file's tag and compare. Two bits of state
make the comparison mean something, and both are recorded when `read` renders:

```go
type Snapshot struct {
    Tag   string       // what we stamped the render with
    Text  string       // exactly what we served
    Lines map[int]bool // the line numbers actually displayed
}
```

`Lines` exists because a windowed or truncated read produces a tag for a file
the model has only partly seen, and an edit outside that window is unreviewed by
anyone. On tag mismatch, attempt a three-way merge against the iteration-start
snapshot; failing that, refuse:

**The refusal branches on whether the file moved, and only one branch may name a
replacement anchor.** The resolver knows which, because it recorded what `read`
served.

```
# the file is byte-identical to what was served: a transcription error
[E_ANCHOR_MISMATCH] There is no anchor `50#HB` in this file. Line 50 is `50#QM`.
Re-issue the REPLACE using an anchor copied exactly from the listing.

# the file changed since the read: the anchor is not the problem
[E_ANCHOR_MISMATCH] This file changed after you read it. Line 50 is no longer
what you saw. Re-read before editing.
  49: for (const e of world.entities) {
  50:   if (!e.alive) continue;
  51:   e.update(dt);
```

Naming the correct anchor in the first branch is safe only because nothing
moved: the file is byte-identical to what was served, so a mismatch can only be
mistranscription and the resolver knows what was meant.

Naming it in the second branch would undo the entire scheme. The anchor exists
to say "line 50 is not what you think"; an error that answers with line 50's
current anchor tells the model to edit content it has never seen, which is the
silent wrong-line edit arriving through the error message instead of through
relocation. **The two cases are indistinguishable from the anchor alone**, so
the decision belongs to the tool and never to the model. The stale branch shows
current content and demands a re-read; it hands back nothing copyable.

This is what `oh-my-pi` does, read from source rather than inferred: it splits
the same two cases on whether the tag resolves to a recorded snapshot, and
neither branch returns a replacement. Its unrecognised-tag message is explicit —
_"never invent the tag and never reuse one from a prior session."_ Its
recognized-tag message says re-read. Ratchet's first branch is the one addition,
and it is licensed only by the byte-identical check that oh-my-pi's coarser
file-level tag does not need to make.

### An anchor must have been minted, not merely be correct

One more precondition, and a measurement is the reason for it. **An edit is
refused unless the anchor it carries was issued by a read in this session**,
even when the anchor matches the file on disk exactly.

Matching the live file sounds like proof enough, and it is not. The stale-branch
message above names the file's current state, so a model can lift an identifier
out of the rejection and retry without ever seeing the new content — and every
check downstream passes, because the identifier is correct. It is correct about
a file the model has not read. Measured, a local model does this on roughly one
refusal in fifteen, and the edits it produced would have deleted a constructor's
`super(id);` and the declaration half of a two-line statement.

The rule is therefore about provenance and not about correctness: Ratchet
records which anchors it handed out and for which lines, and an anchor that came
from anywhere else is refused no matter what it matches. That check costs a
lookup and closes the path by construction, where wording can only discourage
it.

The same reasoning applies one level down, to the lines within an accepted
anchor. Ratchet records which lines a `read` actually displayed and refuses an
edit to any line outside that set, because an anchor proves the model saw _a_
version of the file, not that it saw the line it is editing. Truncated reads,
elided ranges and windowed output all produce anchors whose coverage is partial.

The code is not called stale. "Stale" asserts a cause we do not know, that the
file changed after the model read it. What we know is that the hash does not
match, and the measured split says the cause is usually mistranscription. An
error naming the wrong cause points the model at the wrong fix: told the file
moved it re-reads, told the hash is wrong and here is the right one it copies.
`oh-my-pi` calls the concept a stale anchor; the concept is theirs and the name
here is ours.

The resolver never relocates on a mismatch, however plausible the line number.
That would recover nearly every failure and it is silent relocation, which is
what the anchor exists to prevent.

### The edit pipeline

Four stages, and nothing is written until the last.

```
  resolve anchor ─► apply in memory ─► validate ─► diff-filter ─► write
       │                                     │
       └── E_ANCHOR_MISMATCH                  └── E_EDIT_REJECTED (not applied)
```

Nothing rewrites what the model wrote. An earlier version of this document put a
normalize stage between apply and validate, converting seven dash variants,
eight quote variants and thirteen space variants to ASCII on the grounds that
this is the damage a quantised model does when retyping code. Measured against
19,055 recorded replies, it is not: no model introduced one. Every reply
containing such a character had copied it out of a file that already held it,
and 289 of those scored correct, so the stage would have converted 289 right
answers into wrong ones and each would have looked right in review.

The damage that was measured is indentation. glm reproduced a line correctly and
mangled its leading whitespace 22 times in 30, and 30 replies in 119 across four
models did the same. The repair for that is re-indentation from the line being
replaced, which uses what the tool already knows rather than guessing.

Similarity matching is not in this pipeline and will not be: aider has an
edit-distance matcher at threshold 0.8 and disabled it with a bare `return`, and
an edit applied to code that merely resembles the target is a corruption that
survives review.

Validate runs the language's own checker on the result: `node --check` at
minimum, the attached language server when there is one, which upgrades the
check from "it parses" to unresolved imports and broken references.

Diff-filter is the part nobody replicates and the reason the gate is usable.
Errors present before the edit are re-projected through the line shift and
subtracted, so the model sees only what its change introduced. Without it, a
validation gate on a repo that was not already clean is noise.

A rejected edit returns three things: the error, the text the edit would have
produced, and the file's current content. SWE-agent's ablation is the argument
for all three. Without the error the model misdiagnoses, without its own attempt
it reissues the same edit, without the current content it edits against a memory
from four turns ago.

### Two forms that measured well are not used

`SUB N: OLD => NEW` tops a production-reliability table and is rejected.
Instructed to edit a line that already contains `=>`, the strongest model
available mis-split it and applied a corrupted result 20 times in 26. A
delimiter that can also appear in content is not a delimiter, and the failure is
silent: the form looks like the best one right up to the case that breaks it.

A leading `-` or `+` in content is carried behind a sigil and written once, not
doubled. Across 122 changelog lines beginning at column zero with a dash,
`put_sigil` was wrong 57 times in 200, and one model swallowed the dash 49 times
in 50. One row, valid sigil, correct address, well-formed reply, and the bullet
loses its bullet. No repair reaches that and no syntax gate catches it.
Verifying the replaced row does, which turns the swallow into a refusal and is
why the checked form is the portable one.

### bash

The only tool that enters the container.

```go
exec.CommandContext(ctx, eng.Bin(), "exec",
    "--workdir", "/repo",
    "--env", "PAGER=cat", "--env", "GIT_PAGER=cat",
    "--env", "NO_COLOR=1", "--env", "CI=1",
    "--env", "NPM_CONFIG_PROGRESS=false",
    "--env", "NPM_CONFIG_FUND=false",
    "--env", "NPM_CONFIG_AUDIT=false",
    runID, "bash", "-lc", cmd)
```

The environment is defanged in the image and again per exec, because a `.npmrc`
in the repo can undo the image. Progress-bar sludge is a real fraction of a 32k
window.

Output is capped at 10k characters with head and tail preserved and the middle
elided. Head-only loses the summary line, and build output puts the first error
at the top and the verdict at the bottom.

Empty output returns `ran successfully, produced no output`, because an executor
handed silence starts running commands to find out what happened. One burned 201
tool calls echoing its own status at itself.

## The sandbox

One container per run, created by `ratchet run` and removed when it exits.

```
<engine> run -d --name <runID>
  --mount type=bind,src=<repo>,dst=/repo[,relabel=shared]
  --network ratchet-egress                  # or a proxy; see below
  [--user 1000:1000 | --userns=keep-id]
  --cpus 4 --memory 8g --pids-limit 512
  ratchet-exec:node22
```

The image carries a pinned toolchain and no `git`. Absence is the mechanism: no
binary to invoke, so no policy to circumvent. `revert_file` exists host-side so
the intent behind reaching for `git checkout` has somewhere safe to go.

The repo is the only path mounted. No dotfiles, no `~/.ssh`, no other checkouts,
no host package cache to poison. The motivating case was an executor that,
handed an impossible gate, wrote instructions for adding an alias to the user's
`~/.bashrc`. Not malicious. That was the cheapest remaining way to make a
command called `reviewed` exist.

On macOS, bind-mounted I/O is slow enough that `npm install` notices. Over six
hours, nobody notices.

### Docker and podman, which are not the same thing

Podman is close to drop-in on the verbs, so a binary name covers most of it.
Three things it does not cover, and all three touch a claim made above.

```go
type Engine interface {
    Bin() string
    Run(context.Context, Spec) (id string, err error)
    Exec(context.Context, id string, cmd []string) (Result, error)
    Remove(context.Context, id string) error
    Caps() Caps
}

type Caps struct {
    Rootless        bool
    UIDMapping      UIDMode // userFlag | keepID
    EgressFiltering bool
    SELinuxRelabel  bool
}
```

**UID mapping is the ownership problem again.** Under docker, `--user 1000:1000`
means uid 1000 and a file the container writes lands on the host as uid 1000.
Under rootless podman your host uid maps to container root, so the mount arrives
owned by uid 0 and `--user 1000:1000` cannot write it at all — measured, and the
image's own `USER` line is what triggers it. Dropping the `USER` line makes the
default mapping work, and `--userns=keep-id` makes the `USER` line work.

Neither is what Ratchet uses, because both leave the container with exactly one
of the two things the entrypoint needs. `keep-id` maps the host user to
container 1000 and leaves no root, so nothing can install the egress rules; the
default leaves root but no writable mount for a non-root agent. Explicit maps
give both:

```
--uidmap=0:1:1000 --uidmap=1000:0:1 --uidmap=1001:1001:64000    # and --gidmap
```

Container root exists for the entrypoint, container 1000 is host 1000 for the
mount. The mapping is a capability, not a constant, and the `USER` line is not
independent of it.

**Egress filtering is available rootless, at the cost of a capability.** Rules
in the container's own netns are enforced before packets reach pasta or
slirp4netns, so an allowlist works under rootless podman — measured — but the
container needs `--cap-add=NET_ADMIN` to install it, and that is precisely the
capability an executor must not hold. So the entrypoint installs the allowlist
as root and drops to uid 1000 to run the agent. Verified from inside: egress
blocked, the model host reachable, and `iptables -F` refused.

The remaining gap is what an allowlist can name. A model host is an address. A
package registry is a CDN with a rotating set of them, and `HTTPS_PROXY` is the
fallback there — weaker, because a process that ignores the variable is not
stopped by it.

So Ratchet does not claim containment it does not have.

```
$ ratchet doctor

  config    ~/.config/ratchet/config.json  4 keys · all recognized
  engine    podman 5.4.0 · rootless
  uid       explicit uidmap        container root + host uid 1000 as 1000
  egress    allowlist              NET_ADMIN dropped before the agent starts
            ⚠ registry reached via HTTPS_PROXY; a process ignoring it is not stopped
  image     ratchet-exec:node22    present · git absent · sha256:4c1e…
  mount     /repo                  writable as uid 1000
  models    glm-4.7-flash@block    49152 allocated (asked 49152)
```

The warning line is the point. A sandbox that silently provides less than this
document promises is worse than one that provides less and says so, because the
first kind gets trusted.

SELinux needs the mount relabeled. On Fedora and RHEL a bind mount without
`relabel=shared` is denied by policy, and the denial surfaces inside the
container as permission errors that read like application bugs. Detected once,
at `doctor` time.

Engine selection reads `--container-engine`, then `RATCHET_CONTAINER_ENGINE`,
then `container_engine` in the user config, then the first of docker and podman
present. Whichever wins, `doctor` and `verify` print it with its capability set,
so the same spec on a different machine is never quietly a different run.

## Gates and the mutation sweep

`internal/gate` runs command gates in the container, in order, stopping at the
first failure. The failing command and the last 25 lines of its output become
the retry prompt.

The extractor is what this package exists to get right. It takes only the
`required_commands` section of a `ratchet` block, and it has a unit test that
feeds it a block containing both sections and asserts the ack does not appear.
That test exists because the naive version, which takes every `- ` line, ran
`reviewed` as a shell command and produced a false failure twice in two
harnesses. The second time was because the fix had been made in one script and
never propagated.

### Totality, not absence

`ratchet verify` asserts that the parse of every block is total: every entry
claimed by exactly one field, and by the field matching its section.

The tempting assertion counts what went wrong, `0 acks parsed as commands`, and
it is worthless because a parser that read nothing satisfies it and goes green.
Reconciliation fails on a misclassified ack, on a silently dropped command, and
on a parser returning nothing.

Every gate is then checked for existence with `exec.LookPath` inside the
container, so a gate naming a program that is not there fails at verify time
rather than four hours into a run.

### The sweep

The question is whether an iteration's gates detect the absence of that
iteration's work. If they all pass against a tree where the work was never done,
the gate set verifies nothing, whatever reading it suggests.

That gives one mutation, and it is derived from the spec rather than from any
knowledge of the project:

```go
wt := worktree.New(spec.Tree())        // git worktree add, throwaway
wt.Revert(iteration.Files())           // to their pre-iteration state
for _, g := range iteration.Gates() {
    results[g] = run(g, wt)
}
wt.Remove()
```

**Revert the files the iteration declares, then run its gates.** No language
knowledge, no framework knowledge, no mutation library: the iteration already
says which files it changes, and reverting them is the one perturbation that is
always exactly on point.

An earlier version of this section promised seven generic mutations — append a
stray token, rename an exported symbol, empty a test body, perturb the manifest
— and that was over-generalised from one JavaScript repository. Five of the
seven were npm-shaped and two were hardcoded to a specific file. Mutations aimed
at what a gate checks have to be as project-specific as the gate, and a mutation
library per language is Stryker or mutmut: years of work each, not something
Ratchet ships.

### The assertion is per iteration, not per gate

**At least one gate must fail on the revert.** Not every gate, because gates
come in two kinds and only one of them can carry an iteration:

| Kind          | Asserts                    | On a revert       |
| ------------- | -------------------------- | ----------------- |
| **progress**  | the work happened          | must fail         |
| **invariant** | something did _not_ change | passes, correctly |

`node scripts/check-tests.mjs 31` is a progress gate.
`git diff --quiet -- scripts/check-tests.mjs` is an invariant: it asserts a
guard file was left alone, and a well-behaved executor leaves it alone whether
or not the iteration's real work was done. Requiring that to fail would be
requiring the wrong thing.

So an iteration with no progress gate is the defect. It can be closed by a model
that did nothing, and every gate in it will be green.

The failure must also be substantive; exit 127 does not count. `reviewed` fails
against a reverted tree exactly as a real gate does, because everything fails
when the program does not exist.

### Baseline first, and it is not optional

A fresh `git worktree` has no `node_modules`, so a gate running `npm test` fails
in it for a reason the revert did not cause. Measured on our own spec: four of
iteration 10's seven gates fail in an unmutated worktree, three with
`ERR_MODULE_NOT_FOUND`. Without a baseline the sweep reads those as failing
because of the revert and reports the strongest gates in the spec as the ones
doing the work.

Every gate runs on the unmutated worktree first, and one that fails there is
excluded and reported as **unsweepable**. This is the edit pipeline's
differential filter one level up: subtract what was already failing, then
measure what the change caused.

`worktree_setup` in the repo config populates the worktree — `npm ci`,
`go mod download`, `uv sync`, derived from the lockfile when there is one —
which reduces how many gates get excluded. It is a convenience rather than the
fix; the baseline is what makes running it, or not running it, equally honest.

### What this cannot tell you

It proves a gate set is sensitive to its own work being absent. It cannot prove
a gate checks _enough_, and three real examples from our own spec, all green
through eleven unattended iterations, show the gap:

- `npm ls esbuild` exits 0 with the dependency deleted from `package.json`,
  because it reads the filesystem and not the manifest.
- `check-tests.mjs <min>` counts tests, so forty neutered assertions in
  `entities.test.js` leave iteration 5 green.
- `git ls-files --error-unmatch` and `git diff --quiet` are a defensive pair,
  untracking and modification, and a spec carrying one of them has a hole.

Finding those took mutations written against this project's own idioms. Ratchet
reports the limit rather than implying otherwise: a spec can pass the sweep and
still be checked by gates that verify less than they appear to, and that
judgment stays with the human reviewing the plan.

### One mutation, so no matrix

An earlier version of this section described reading the gate-by-mutation matrix
by column: a mutation that kills nothing names a failure mode the whole gate set
misses. That reading is real and it is how the assertion-removal hole above was
found — but it belongs to a mutation library, and there is one mutation here.

It is worth keeping as a note for anyone who does write project-specific
mutations: run them, and read the columns as well as the rows. A column of
zeroes is more informative than any row.

## The index

Built on demand, cached by tree hash, never watched.

```go
func (ix *Index) For(ctx context.Context, tree string) (*Snapshot, error) {
    if s, ok := ix.cache.Get(tree); ok { return s, nil }
    s := build(ctx, ix.providers, tree)
    ix.cache.Put(tree, s)
    return s, nil
}
```

A file-watching daemon suggests itself and is wrong. During a run the executor
mutates files constantly, so a watcher spends the run invalidating an index
whose consumer is idle, and cross-boundary inotify from a bind mount is
unreliable on macOS. Building on query cannot serve a stale answer, because a
changed tree is a cache miss.

```go
type Provider interface {
    Languages() []string
    Graph(root string) ([]Edge, error)
    EntryPoints(root string) ([]string, error)
    Commands(root string) (setup, build, test string, err error)
}
```

The JS/TS provider reads esbuild's metafile, which is faster and less wrong than
regexing import statements. Where a language server is available it is attached
and its `textDocument/references` answers queries directly.

The executor's query surface is narrow on purpose: `who imports <path>`,
`does <module> export <symbol>`, `where is <symbol> defined`. The drafter gets
the whole snapshot; the executor gets replies. A dump is 6k tokens and a reply
is forty, and the executor has 32k and no memory.

The index holds only what is derivable from code. Judgments about the code go in
the iteration that needs them or in `AGENTS.md`. There is no third file of
machine judgments, because those go stale plausibly and cannot be falsified
without redoing the work behind them.

## The drafter's server

`ratchet plan` binds a listener, prints the URL, opens a browser and returns.
The server outlives the command.

```
GET    /                                          the new-spec form
POST   /specs                                     create a spec from an intent
GET    /specs/{spec}                              the drafting page
GET    /specs/{spec}/events                       SSE: progress, questions, revisions
GET    /specs/{spec}/threads                      every thread on this spec
POST   /specs/{spec}/threads                      open a thread on an iteration
POST   /specs/{spec}/threads/{thread}/comments    add a comment
PUT    /specs/{spec}/threads/{thread}/resolution  answer, or pick
GET    /specs/{spec}/mockups/{mockup}/{variant}   a variant, as its own document
```

**The spec is the resource, so it is `/specs`, spelled out and plural.** An
abbreviation saves five characters once and costs a guess every time someone
reads the route.

`POST /specs` creates it, which removes a state the alternative allows: a
session open against a spec that does not exist. The id is allocated at
creation. `ratchet plan` with no argument opens the form; `ratchet plan "…"`
posts the intent itself. The CLI is a client of its own server for that call, so
there is one creation path rather than two that drift.

The intent is immutable, so no route updates it. It is the premise the spec was
written against, and a premise you can quietly revise is not a premise. Nothing
has been built yet, so the escape hatch is a better intent or a thread.

**Everything the human and the drafter say to each other is a thread.** A
comment, a question, a choice and a mockup differ by kind and by how they
resolve, not in structure: each anchored to an iteration, each accumulating
comments, each carrying at most one resolution.

```
POST /threads/{id}/comments     append a child        many, ordered
PUT  /threads/{id}/resolution   set the one answer    idempotent, overwritable
```

Separating append from set makes the verb carry the meaning, and it collapses
"answer a question", "pick option A" and "pick variant B" into one route whose
body differs. Re-picking before approval is a second `PUT`.

Mockup variants stay a separate collection because the artifact and the
conversation about it are different things. The thread resolves by naming a
variant; the variant is a document at a short URL, opened at a real viewport.

The drafter does not use any of this. The HTTP surface is the human's half.
Questions, decisions, choices and mockups arrive through `drafter/tool`, so
there is no route for raising one, and nothing holding an HTTP client can
pretend to be the drafter.

Server-sent events rather than websockets. Traffic is one-directional and low
volume, SSE reconnects by itself and survives a laptop sleeping, and a websocket
would add a framing layer to carry strictly less.

Threads live in the journal as `question`, `decision`, `thread`, `choice` and
`mockup` events. The page is a view over an append-only log rather than a store
of its own, so there is nothing to reconcile between what the page shows and
what happened.

Threads anchor to iteration ids, never line numbers. A revision that reflows the
document orphans every line-anchored comment, and an id survives the iteration
being rewritten entirely, which is when the comment still matters.

The server binds to `127.0.0.1` by default. `--listen` takes an address for the
common case where the repo is on one machine and the browser is on another,
because the machine with the GPU is usually not the machine with the screen.

It stops at approval. There is no run dashboard: once a spec is running there is
no human in that half of the system, so `ratchet status` and notifications carry
it and neither needs a live process.

### The page

Go `html/template` on the server, htmx for the interactions, TypeScript compiled
by esbuild's Go API at startup.

The reason for server rendering is that **the server already owns every piece of
state the page displays.** The spec is a file, threads are journal events,
status is a fold, headroom comes from the index. A client framework would hold a
second copy and reconcile it against SSE, and a second copy that can disagree
with the first is the failure this project is organized against.

The interactions are the wrong shape for a component tree anyway. Every one is
post a small thing, get a fragment, swap it in. htmx is fourteen kilobytes and
expresses that, with SSE as an extension.

Server rendering keeps the page in the existing test suite. Rendered HTML goes
through the same golden fixtures and `-update` flow as the CLI tables, in the
same `go test -race ./...`. That extends to the dependency diagram, emitted as
SVG from Go rather than drawn by a JavaScript library, so the diagram is a text
fixture and a change shows up in a diff.

#### The client code is TypeScript, compiled at startup

It starts small: text selection for anchoring a comment, and the diff expander.
It will not stay small. An htmx page accumulates JavaScript one reasonable
addition at a time, and the point at which you want types is well before the
point at which you notice you needed them.

```
internal/drafter/session/web/
  src/*.ts               authored, and embedded as source
  src/*.css
  vendor/htmx.min.js     pinned, with its version and digest recorded
```

**There is no `dist/` and no build step.** esbuild is a Go library, so the
binary embeds the TypeScript and compiles it when the server starts.

```go
//go:embed web/src web/vendor
var webFS embed.FS

func (s *Server) buildAssets() error {   // once, at listen time
    r := api.Build(api.BuildOptions{
        EntryPoints: []string{"web/src/main.ts"},
        Bundle: true, MinifySyntax: true, Sourcemap: api.SourceMapLinked,
        Target: api.ES2022,
    })
    ...
}
```

This is free only because esbuild is already linked for the index's module
graph. Adding ten megabytes to the binary purely to avoid a build step would be
a trade worth arguing about.

Not committing the output deletes a failure mode. A checked-in artifact is a
second copy of the client code: edit a `.ts`, forget to rebuild, commit, and the
shipped page is silently the old one. Guarding that needs a source digest, a
freshness test and a `make web` nobody must forget. Derived data should not be
committed because it can be regenerated, and this document says so about the
index.

```go
func TestWebSourcesCompile(t *testing.T)   // build the embedded TS, assert no errors
```

A syntax error becomes a `go test ./...` failure rather than a server that will
not start, which is the one thing moving compilation to runtime could have cost.
The test needs only Go.

Types need a separate pass, because esbuild erases them. `make web-check` runs
`tsc --noEmit`, the only thing in the project that wants node. A contributor
without node can build, test and ship; they cannot typecheck, and CI can.

#### What would overturn the server-rendered part

If the spec became editable in the browser rather than in your editor, the page
acquires real client state, undo and conflict handling, and that is a case for a
client framework. Worth revisiting the day it is on the table. Editing a file in
the editor you already have is not a feature gap.

The TypeScript pipeline is independent of that decision, which is why it is
worth building before it is needed.

Mockups are outside all of this. They are arbitrary HTML the drafter wrote,
served at their own URLs. They are never embedded in the page: the first
implementation used an iframe and the iframe lied about width, which is the one
thing a mockup exists to tell you the truth about.

## Concurrency, locking and crash recovery

**One writer per spec.** `ratchet run` takes an exclusive `flock` on
`.ratchet/runs/<spec>/lock` for the whole run. A second `run` against the same
spec fails immediately and says which pid holds it. Two runs sharing a working
tree is a race with no correct outcome.

The reader takes the same lock, shared. That stops the class of bug where a
scorer reads a half-written log and reports a failure that never happened. It
happened twice.

Commits are the recovery points. The tree is committed before an iteration
begins and after it passes, and between them a diff is snapshotted after every
operation, so a process killed mid-iteration leaves the work in
`.ratchet/runs/<spec>/snap/`.

Startup reconciles. `run` and `resume` compare the state block against the tree
by replaying every completed iteration's gates against `HEAD`. Where to restart
is the highest `N` for which every iteration up to `N` currently passes, not the
last iteration carrying a `closed` row. Those agree in the ordinary case and
diverge exactly when something has gone wrong, and then the tree is the
authority.

Correction is forward-only. A record that disagrees with the tree is never
rewritten. Ratchet appends `reopened` with the reason and folds both, because an
append-only log you are willing to edit is a mutable field with extra steps.

External changes are detected, not absorbed. The runner records `HEAD` when an
iteration begins, and if `HEAD` moves without the runner moving it the attempt
is abandoned and restarted from wherever the tree now is, journalled as
`external_change`. Misattributing that to the model costs hours debugging
something that was behaving perfectly, and we have paid it.

Session ids are unique per run. `pi` restores a session's model _and_ its
history from a session id, so a reused id silently inherits the previous run's
context window and its poisoned transcript while the console reports the new
configuration. The id is `<spec>-<iteration>-<runid>`.

## Notifications

```go
type Channel interface {
    Send(ctx context.Context, m Message) error
    Kind() Kind    // text, voice
}
```

Messages are short by construction: the type carries one headline and an
optional detail block, and there is no multi-paragraph form to reach for. A
five-paragraph status update on a phone is indistinguishable from silence.

Sends are best-effort and never block the run. A channel that fails logs to the
journal and the iteration continues, because a notification failure is not a run
failure and treating it as one means a dead webhook can stop a migration.

## Testing a system whose main dependency is stochastic

The model cannot be asserted against. Everything around it can, and that split
is the strategy.

### Replay

A journal is a complete record of what the model saw and said. Point the
provider at one and the loop is deterministic.

Assertions use **gomega**, in plain `go test` functions rather than under
ginkgo: `NewWithT(t)` at the top, `Expect(...).To(...)` after it. The matchers
read as the claim being made, and a failure prints the value rather than only
the line.

```go
g := NewWithT(t)
p := journal.Replay("testdata/glm-i6-a2.jsonl")
run := agent.New(p, tools, gates)
g.Expect(run.Iteration("i-9e2b14").Result).To(Equal("PASS"))
```

Cassettes come from real runs against real models rather than responses somebody
imagined. When a bug is found in the loop, the run that exposed it becomes a
test: the 201-echo run, the truncation cascade and the ack-as-command false
failure are all cassettes.

Replay is strict. A mismatch between the request the loop produces and the
request the cassette recorded is a failure, not a fallthrough to the live model.
That is what makes a refactor of the prompt builder detectable.

### What gets unit tests

The deterministic majority. `internal/anchor` round-trips render and resolve
over generated files including adversarial ones: identical adjacent lines,
one-line files, files ending without a newline, lines containing `#`.
`internal/spec` parses every spec in `testdata/` and asserts totality, including
the block with both sections the extractor once got wrong. `internal/gate` runs
against fixture repos with known-broken trees. `classify` is a table test, one
row per failure observed in the wild, each citing the run it came from.

### Failure injection

Rule six says make every new gate fail on purpose once before trusting a pass
from it, so the seams are explicit.

```go
type Faults struct {
    DropToolCall    float64   // serving layer eats the call, returns prose
    TruncateAt      int       // cut the stream after N tokens, mid-call
    EmptyGeneration bool      // stop: length, no content
    LieAboutFinish  bool      // report "stop" on a truncated response
    NetworkFlap     float64
    ContextShrink   int       // allocate less than requested, silently
}
```

Every field is a thing a real serving stack did to us. `ContextShrink`
reproduces the 4096-instead-of-48k case, `LieAboutFinish` the local servers that
report a clean stop on a cut-off response, `DropToolCall` the parser that turns
a tool call into text. The suite runs the full loop under each and asserts the
failure is classified correctly, reported by kind, and recoverable.

Which is how we know the harness handles them rather than believing it does.
Nineteen times a defect in the instrumentation looked like a defect in the
model, so the instrumentation gets the adversarial tests.

### What is not tested automatically

Whether a spec is any good. That is the human's job at review time. The sweep
proves a gate is sensitive to something; it cannot prove the gate is sensitive
to the thing that matters.

## Performance, where it matters

Almost nowhere. An iteration runs for minutes to hours bounded by decode speed,
so Ratchet's own CPU time is noise.

Three exceptions, all about a human waiting. `ratchet list` reads the tail of
each spec rather than the whole file, because folding N state blocks must feel
instant. The index build is a few hundred milliseconds, cached by tree hash. And
page updates go out as events arrive rather than on a poll, because a progress
display that lags teaches you not to trust it.

## What we chose not to build

A daemon. Nothing runs between commands except the drafting server, and that
dies at approval. A background service would need lifecycle management, log
rotation and a story for what happens when it is not running, in exchange for
saving an index build measured in hundreds of milliseconds.

A plugin ABI. Providers, tools, index languages and notification channels are
interfaces and all implementations are compiled in. The surface is still moving,
and a stable ABI over a design we are still learning would freeze the wrong
decisions.

A database. The access patterns are a log and a blob.

An LLM judge. A judge wrong half the time makes three attempts worse than one.
The scorers are deterministic: the gates pass, the tests pass, the diff is
non-empty. If that cannot express your definition of done, the missing piece is
an ack gate and a person.

Similarity-matched edits. Whitespace and Unicode normalization yes. Fuzzy
matching no, for the reason every mature editor reached independently: an edit
applied to code that merely resembles the target is a corruption that passes
review.

A run dashboard. There is no human in that phase to watch it.

## The shape of the whole thing

Ratchet is a supervisor with strong opinions about failure, in one binary,
wrapped around a network call it does not trust and a shell it does not trust
either.

The model is a service. The container is a shell sandbox. Git and the specs stay
on the host, in files you can read. Everything derived is a cache you may
delete. Everything asserted is a row somebody appended.

Nothing in it is clever. The measurements said the clever part was never the
problem.
