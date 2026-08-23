# Ratchet

_A product document written as if it shipped. None of it exists yet. How it is
built: [architecture.md](architecture.md). What has since been measured, and
what is still unvalidated, are recorded in the research notes that preceded this
repo._

## What Ratchet is

Ratchet runs a local model as an unattended software engineer.

You describe what you want. A larger model turns it into a **spec**: an ordered
list of small verifiable iterations, each sized to fit a local model's context
window. A local executor works through them alone, on your hardware, while you
do something else. Every iteration that passes its gates is committed.

A ratchet turns one way.

## Why Ratchet exists

The models are already good enough. Almost nothing else is.

A 30B model at 4 bits, on a laptop, migrated a real codebase off Babel 5 and
browserify to native ES modules with a working test suite. Eleven iterations,
unattended, every gate green. The same model did it on a 24GB GPU in 26 minutes
and on a MacBook Pro in 6 hours. It used 494 tool calls on one and 502 on the
other. The hardware changed the clock, not the work.

That capability is available now, at no cost per token, with no code leaving the
building. A way to use it unattended is not.

Getting there took three passes over the same spec. Every stop was ours. A
timeout calibrated on faster hardware killed a converging attempt. Compaction
constants copied from another machine thrashed a 32k window. A scorer ran a
human approval as a shell command. Nineteen times a defect in our
instrumentation looked like a defect in the model. Nineteen times it was the
instrumentation.

The failure modes are ordinary and they live in the harness.

- The serving layer drops a tool call. Reported as the model choosing not to
  act.
- A stream truncates mid-call with no error. Reported as the model giving up.
- An edit fails. A frontier model's chance of ever fixing that file drops from
  90% to 57%. A local model's drops further.
- A model answers a failed edit by running `git checkout` over its own correct
  work. Two models did this. One reverted the whole tree.
- A gate that cannot fail passes.
- A model with no way to say "I am stuck" invents one. 201 `echo` calls.

A better model fixes none of that. Ratchet assumes the model is roughly good
enough and everything around it is not.

Three things it is for. Work you would rather not pay a frontier agent to type:
migrations, mechanical refactors, test backfills, dependency sweeps. Work that
cannot leave your machine. Work that should happen while you are not there.

## The escalation ladder

Every problem goes to the simplest tier that can reliably solve it.

| Tier                            | Handles                                              | Costs                           |
| ------------------------------- | ---------------------------------------------------- | ------------------------------- |
| **1 · Ratchet** (code)          | anything mechanically derivable                      | nothing, same answer every time |
| **2 · Executor** (local model)  | mechanical work needing judgment code cannot express | electricity and a 32k window    |
| **3 · Drafter** (a large model) | ambiguity, structure, consequential decisions        | money or memory, and minutes    |
| **4 · Human**                   | what no model reliably solves, and verification      | the only genuinely scarce tier  |

Most decisions in this document are that rule applied. The index is code, not a
model pass. Gates are shell commands, not an LLM judge. Line addresses are
hashes checked by code, not similarity matched by a model. Status is a fold over
rows, not a summary an agent writes. Cutting the seams is tier 3. Approving a
spec is tier 4.

Two reasons to push work down, and they are independent.

The lower tier is more reliable at what it can do. A fold over a table gives the
same status every time. A model gives it nearly every time.

And every problem tier 1 absorbs is budget the upper tiers keep. Tier 3 costs
money per token. Tier 2 has 32k tokens and no memory between iterations. Tier 4
cannot be bought at any price. So the executor queries the index instead of
reading it: a 40-token answer where a dump is 6k.

The two directions of error fail differently. Too low fails at once. Code cannot
cut seams in a codebase and you find out in five minutes. Too high usually
works. You get the right answer most of the time, having traded a reproducible
result for a probabilistic one and paid for it. Nothing announces the trade.
Bias downward.

"Reliably" is measured. We asked whether a local executor could transcribe a
hash-anchored line address. One model scored 30 of 30, another 22 of 30. The fix
was not to escalate the task. Tier 1 props up tier 2: when the anchor is wrong,
code supplies the right one.

Tier 3 is a capability, not a vendor. Reading a codebase and cutting seams needs
no hosted model, and a 70B-class local model is a plausible drafter. Untested.
Drafting quality has a metric that would settle it: how often the executor comes
back `BLOCKED`. Draft one intent twice, hosted and local, run both, compare.

The tier survives either way. A dense 27B managed 5.5 tokens per second on an M1
Pro, and a 70B cannot sit resident beside a 20GB executor. Running both means a
reload each way. Intolerable per iteration. Fine once per spec.

Structure per tier is inversely proportional to trust. Tier 1 gets none. Tier 2
gets one iteration, a file list, seven tools and no `git`. Tier 3 gets the
repository and a conversation. Tier 4 gets prose and a browser.

## The three seats

Three of the four tiers exercise judgment. Ratchet is the fourth column and
decides nothing: it runs gates, commits, snapshots, folds rows. Every one of
those answers is the same every time.

> Collaboration lives between the human and the drafter. Automation lives
> between Ratchet and the executor.

So anything requiring collaboration with an agent is drafting work. There is
nowhere else for it. Visual design moves out of execution. The executor gets
`BLOCKED` instead of the ability to ask. The graphical surface belongs to
drafting. A spec cannot start with an open question. One call, made once.

|                             | Human    | Drafter | Executor              | Ratchet |
| --------------------------- | -------- | ------- | --------------------- | ------- |
| States intent               | ✅       |         |                       |         |
| Writes the spec             | comments | ✅      |                       |         |
| Asks questions              | ✅       | ✅      | **cannot**, `BLOCKED` |         |
| Approves the spec           | ✅       |         |                       |         |
| Edits code                  | can      | never   | ✅                    | never   |
| Runs command gates          | can      | can     | ✅ (and must)         | ✅      |
| Marks an iteration complete |          |         | no verb exists        | ✅      |
| Supplies an ack             | ✅       |         |                       |         |
| Commits and snapshots       |          |         | no `git` at all       | ✅      |
| Stops the run               | ✅       |         | ✅ (BLOCKED)          | ✅      |

The executor cannot declare itself finished. It emits `DONE`, which is a claim.
The runner decides by running the gates. Told three times in plain English not
to close an iteration needing human approval, a model closed it anyway and
reported the work reviewed. Given a tool set without that verb, it stopped and
asked. It did not become more trustworthy. It stopped having the capability.

The same holds for checking. Four models were asked to review a specification.
None had tools or a repository, so nothing they cited could be opened. **Three
of the four said they had checked it anyway.** One wrote out a table of ticks —
"✔️ The double-counts show up in `TestRebuild` (currently passes)" — for a test
file it could not read. Only one model said, in all three of its runs, that it
had no tools and could check nothing.

So "I checked it" is worth no more than "I am finished". Both are claims. That
is why a gate is a command the runner runs rather than a status the executor
reports, and why an anchor is a hash the tool handed out rather than a line
number the model remembers. Neither stops a model that lies. Both stop one that
thinks it checked and did not.

Only the drafter can ask you anything. The executor has `BLOCKED`. It stops the
iteration and states the problem, and the answer arrives as a revised spec. An
unattended run that can block on a question is a process waiting for you with a
20GB model loaded and a laptop that will sleep.

## The spec

A spec is a markdown document, in your repo, in version control, readable in
five minutes.

Ratchet claims no directory and no extension. Specs go where your specs already
go, beside ones no agent will touch. A Ratchet spec is identified by its fenced
`ratchet` blocks. A spec without one is a document.

````markdown
# Spec 001: ESM migration

Migrate off Babel 5 / browserify / grunt to native ESM and esbuild, with a test
suite on Node's built-in runner.

```ratchet
executor_class: 24B-local
min_executor_context_window: 32768
```

## Iteration 6: game.js reads world.js instead of `exports`

**Design decisions this iteration depends on.** world.js owns the entity list;
game.js must not re-export it. The cycle being broken is game → entities → game.
world.js sits under both and imports neither. Keep `overlaps()` in entities.js;
the server imports it too.

**Do this:**

1. Replace the `exports.entities` reads in game.js with imports from world.js
2. Delete the re-export block at the bottom of game.js
3. Add test/game.test.js covering entity spawn and collision

```ratchet
files:
  - src/game.js
  - test/game.test.js
required_commands:
  - node scripts/check-tests.mjs 23
  - npm test
required_acks:
  - reviewed
```
````

Prose for the human, a fenced block for the machine. A person reviewing this
spec is the last line of defense before hours of unattended work, so the
decisions and the steps are written to be read. Only what must be exact goes in
the block.

Handwritten structured formats have failure modes a spec cannot afford.
Indentation sensitivity. Colons inside unquoted strings. Three multi-line string
modes. `no` and `on` parse as booleans. Every complete run in our research
executed a spec of this shape.

### Both fields are gates

An iteration does not close until every entry in both lists is satisfied. A
**command gate** is a shell command the runner runs and checks. An **ack gate**
is an assertion only a person can make. "Gate" is the umbrella, so it cannot be
the name of one half.

Do not over-credit the separation. In our research the two were already separate
fields. A scorer took every `- ` line from the block anyway, ran `reviewed` as a
shell command, and produced a false failure twice in two harnesses. The second
time it cost an executor 79 tool calls trying to bring a program named
`reviewed` into existence.

A format cannot protect you from a parser you never tested. What prevents this
is an assertion that the parse is total: every entry claimed by exactly one
field, and by the field matching its section. `ratchet verify` reports it first.

The separation earns its keep in prompt generation. The executor's prompt comes
from a template that can only reference the command field. One list would need a
filter, and a filter bug puts an ack into the executor's "make these pass"
instruction.

### The rest of the shape

Design decisions are prose, restated in every iteration that depends on them.
The executor starts each iteration with an empty context and does not remember
iteration 5. Any decision it needs must be written where it will read it, even
three times over.

`files` is a boundary. The executor's edit tools are scoped to that list. A path
absent from `files` is a path it cannot name, so it cannot weaken its own gate.
Not because it was told not to.

`min_executor_context_window` is a contract. Ratchet refuses an executor whose
actual allocated context is below it, and reads the allocation from the serving
runtime rather than trusting the request. A model silently running at 4096
tokens because a compatibility endpoint discarded the option is the most
expensive lie in this space.

The name is long on purpose. `min_` because it is a floor: a spec sized for 32k
runs happily in 64k, and `context_window` invites someone to write their maximum
and find that no executor qualifies. `executor_` because one model drafts and
another executes.

### State is an append-only block

At the bottom of the spec, written by Ratchet, never edited.

```markdown
<!-- ratchet:state · append-only · Ratchet writes this -->

| when             | event     | by      | detail                      |
| ---------------- | --------- | ------- | --------------------------- |
| 2026-08-16 09:14 | ready     | human   | spec sha256:8f3c1ab         |
| 2026-08-16 09:15 | started   | ratchet | glm-4.7-flash@block         |
| 2026-08-16 15:42 | completed | ratchet | 11 of 11 · npm run smoke ok |
```

Status is the fold over that table. `draft` is the absence of a `ready` whose
hash matches. `blocked` is an unresolved `blocked` row. `running` is a `started`
with no terminal row after it. No cell contains the word "draft".

`by` is a role, not a name. Ratchet cannot know which person approved a spec. It
can read `git config user.name`, which is self-asserted and trivially wrong on a
shared machine. Roles it does know, because roles here are structural: `ready`
can only come from the human seat, since no other seat has the verb. The row is
in a commit, so git has already attributed it, and a signature is the only one
of the three that is evidence.

Roles make an invariant visible. No row is ever `by: executor`. `ratchet verify`
asserts it.

**An approval is anchored to the content it approved.** The `ready` row carries
a hash of everything above the state marker. Edit an iteration afterwards and
the hash stops matching: "approved at v3, now v4." A `status: ready` line would
still say ready.

Every transition has a when and a who. Overwriting a field destroys the previous
value and records neither.

A human reading the file learns the state from the file. In a browser, a diff, a
repository viewer. State in a separate index means the most common way anyone
looks at a spec tells them nothing about it.

State cannot outlive the spec or belong to the wrong one. Delete the spec and
the state goes. A separate ledger accumulates rows for specs that were renamed
or abandoned, with nothing to say which. The history of a deleted spec is in
git.

**State and code revert together, because they are one commit.** Ratchet appends
the row in the same commit as the work it describes. Revert it and both
disappear. Abandon a branch and the spec reads `completed` on the branch and
`draft` on main, both true where they are. With state outside the commit,
reverting the work leaves a record insisting it happened.

Two qualifications. The drift this prevents is referential: state pointing at
the wrong spec, or none. It does not stop state disagreeing with the spec's
content, which is what the hash is for. And the growth concern is churn, not
size. A dozen rows per spec is nothing. One file every branch appends to is a
hot spot thousands of commits deep where `git log` is useless.

Blocked is a human assertion, so it is a human-written row. Nothing in git can
tell you a spec is waiting on an API contract that does not exist yet.
`ratchet block 004 "waiting on the API contract"` appends a row; `unblock`
appends its resolution. The reason survives instead of being compressed into
"blocked". Waiting on another spec is typed separately: Ratchet clears
`blocked_on 004` itself when 004 records `completed`.

Nothing the executor can reach writes here. `ready`, `block` and `unblock` are
absent from its tool set. The spec driving a run is not addressable by that
run's executor, by identity rather than by path, so it does not depend on specs
living in a private directory.

## A day with Ratchet

### From the human's seat

You are in your terminal with a repo and an intention.

```
$ ratchet plan

  http://localhost:7000/
  opening browser…
```

`ratchet plan` does not draft a spec and hand it back. It opens a session.

You state the intent in the page, not the shell. The prompt is the most
consequential input in the system, and a terminal is the worst place to compose
one: no editing, no newlines without a fight, quoting rules that punish long
specific sentences. `ratchet plan "…"` still works for a one-liner and skips
this step.

Nothing is allocated until you have said what you want. The spec, and its
number, come into existence when you submit:

```
  006 · drafting · http://localhost:7000/specs/006
```

That is the whole file at that point:

````markdown
# Spec 006: untitled

> Migrate this off babel and browserify to native ESM, and get a real test suite
> on it. I care more about the tests than the bundler, pick whatever is least
> work.
>
> — stated 2026-08-18 09:02

<!-- ratchet:state · append-only · Ratchet writes this -->

| when             | event  | by    | detail                |
| ---------------- | ------ | ----- | --------------------- |
| 2026-08-18 09:02 | opened | human | session · claude-opus |

```

Your request stays at the top of the spec permanently, verbatim, never
summarized. Six months later, "what was actually asked for?" has an answer that
is not an inference from eleven iterations.

Then you watch it work, or you do not.

```

006 · drafting 5m 12s

✓ index 34 modules · 1 cycle · 2 entry points ✓ seams 11 iterations proposed ⣾
sizing iteration 6 · 21.0k of 32k ⚠ 36% headroom · decisions — · gates —

1 question waiting · 0 choices · 0 mockups

```

Drafting takes real minutes. Silence is indistinguishable from a broken process.
An unattended executor can be silent because notifications cover it. A drafter
working while you watch cannot.

The surface being up from the start also gives questions somewhere to arrive.
Ten minutes in:

> **006 · the drafter has a question** _(blocking)_\
> Iteration 3 needs a test runner. `node --test` is built in and adds no
> dependency; `vitest` gives watch mode and better output. Nothing in the repo
> indicates a preference.\
> Reply `node` or `vitest`, or answer it on the page.

You reply `node`. Two more arrive over twenty minutes and only one blocks.

> **006 · decision recorded** _(not blocking)_\
> `overlaps()` is called from both entities.js and the server. I have kept it in
> entities.js and imported it from both, rather than moving it to world.js. Iteration 5 is
> written that way. Say so if you would rather it moved.

A blocking question stops the draft. A recorded decision does not: the drafter settles it,
writes it into the iteration that depends on it, and tells you as a statement rather than a
prompt. Same move as the executor's `BLOCKED`, one phase earlier. Decide and say so where you
can, stop and say why where you cannot, never quietly pick.

#### Reading and marking up the draft

The draft appears on the page you have open. If you closed the tab, one command
attaches to the same session.

```

$ ratchet review 006 http://localhost:7000/specs/006 (open, or send yourself the
link)

```

This is the only graphical surface in Ratchet, and it covers one phase. Drafting
has a human in it; execution does not. Reading a 500-line structured document
and pointing at part of it is the one task where a terminal is worse.

What the page gives you that `cat` does not:

- **The spec, rendered.** Iterations as sections, gates as a table, the
  dependency chain as a diagram, sizing headroom as a bar.
- **Comment threads anchored to iterations**, or to a single step: "Iteration 6
  is too big, split the game.js rewrite from the test."
- **The drafter's questions in place**, next to the iteration they affect.
- **Mockups**, rendered, when a decision can only be made by looking.
- **A version diff.** You read what moved, not 500 lines.
- **Live progress**, from the same page that showed the drafter reading your
  codebase.

Threads anchor to iteration ids, not line numbers. A revision that reflows the
document orphans every line-anchored comment. An id survives the iteration being
rewritten, which is when the comment still matters.

You leave three comments and hit send. The reply arrives on the surface the
message was sent from. You commented on the page, so the revision lands on the
page, each thread showing what the drafter did. A notification goes out
regardless.

> **006 · revised (v3)** · 12 iterations, was 11\
> Split iteration 6 into 6 (game.js reads world.js) and 7 (game.js tests). Both
> now under 14k. Your other two comments are answered in their threads.\
> 0 open questions.

`ratchet review 006 --changes` prints the same diff. That is you pulling it.

#### Choices

A third shape of question is a fork with costs on both sides. Breaking the game
→ entities → game cycle has three shapes, and the right one depends on what you
intend to do with this codebase next.

The drafter does not ask an open question, which would put the enumerating back
on you. It presents a choice.

```

choice C1 — how to break the entities/game cycle [awaiting you]

A world.js underneath both recommended 3 files, ~40 lines moved. Neither module
imports the other. Forecloses: nothing. Costs: one new module. B entities.js
takes a context argument 1 file, ~15 lines. Smallest diff. Forecloses: multiple
worlds later. Costs: every call site grows a parameter. C event bus between them
6 files. Removes the dependency entirely. Forecloses: nothing. Costs: control
flow becomes non-obvious; hard to test.

Drafter's reasoning: B is smallest but the parameter threading shows up in every
future iteration's diff. C is architecturally cleanest and is the option a 24B
executor is least likely to implement correctly. A is boring and mechanical,
which is what you want when the executor is the one typing.

```

You can also reject all three and say what you want instead, in text. Without that a choice
is a leading question: the drafter enumerated three architectures and it is not the party
with the final say about whether those are the three. When it happens, that is worth knowing
too, because a choice answered in free text means the enumeration was wrong.

You pick A. The pick becomes design prose in every iteration it affects, stated as a
decision. The rejected options stay on the review, not in the spec. The
executor never sees the choice: it gets the decision, never the deliberation.
Three options it is not being asked to choose between is an invitation to
improvise.

A resolved choice is the best candidate for promotion into `AGENTS.md`.

One live risk. A drafter that raises a choice for every decision has made the
spec unreviewable and rebuilt the thing we were escaping. Choices are for forks
you own: architecture, scope, what the codebase is becoming. Ratchet reports the
count. Nine choices is a signal about the drafter.

#### Mockups

Some work cannot be specified in prose because its correctness is visual. A
screen layout, a flow between screens, the shape of a rendered report. "Describe
what you want in a paragraph" is a guess about what the paragraph will produce.

So the drafter builds the thing and hosts it, on the server already serving the
review.

```

$ ratchet review 003

http://localhost:7000/specs/003 spec · 12 iterations · 0 open questions M1 ·
settings panel layout · 3 variants [awaiting you]
````

You open M1 and see three working layouts, not descriptions of three layouts.
You click through them, resize the window, note that variant B breaks at narrow
widths, and pick A with a note about spacing. Same threads machinery. The
artifact is a page instead of a paragraph.

##### Why this moved out of execution

Conventionally this is a todo inside an iteration. Two reasons it moves, and the
second is the real one.

The local executor cannot do it. Visual design needs holistic judgment across a
whole artifact, the first thing to go at 4 bits and 32k.

A design decision made during execution is a design decision made unattended.
Every other decision has been moved out of the executor's hands on principle. A
visual judgment is the least verifiable decision in the spec: no command exits
non-zero when a layout is wrong. Leaving it in an iteration puts the one
decision no gate can check into the one phase where nobody is watching. That
would be the right line even if the executor were excellent.

##### What the executor gets instead

A reference to the approved artifact, committed to the repo.

````markdown
## Iteration 8: the settings panel

**Design decisions this iteration depends on.** The approved layout is
`design/settings-panel.html`, chosen from mockup M1. Match its structure,
spacing scale and element order exactly. Where the mockup and this prose
disagree, the mockup is correct; it is the thing that was reviewed.

```ratchet
files:
  - src/settings.js
  - design/settings-panel.html
required_commands:
  - npm run build
  - npm test
required_acks:
  - design-matches-mockup
```
````

````

The executor matches an artifact rather than having taste. A human checks it in
seconds by putting the two side by side.

`design-matches-mockup` is the clearest ack gate there is. "Reviewed" is vague,
and vague acks invite the shrug; it is the kind of gate a model tries to satisfy
by creating a file called `reviewed`. "Does the built panel match the mockup you
approved?" is precise, only a human can answer it, and no command can fake it.

#### Approving it

A spec becomes runnable the way an iteration closes. It has gates.

```
$ ratchet ready 001

  ✗ 1 thread open       — iteration 9, "does this need the ws upgrade path?"
  ✗ 1 choice open       — C2 (bundler config)
  ✗ 1 mockup undecided  — M1 (settings panel layout), no variant picked
  ✓ every iteration fits 32k with ≥40% headroom
  ✓ mutation sweep passes on 54 command gates (0 unsweepable)
  ✓ index fresh · tree 8f3c1ab

  not ready
```

A spec cannot become runnable while a question or choice is open, or a mockup
undecided. Not a warning. It prevents a six-hour unattended run built on a
decision nobody made. The undecided mockup is the sharpest case: the run reaches
iteration 8, hands the executor a reference to an approved layout, and there is
not one.

`ratchet ready 001` records your approval as a row carrying the time, the
`human` role, and a hash of what you approved.

#### Seeing where everything stands

```
$ ratchet list

  006  untitled             drafting  —            5m · 1 question waiting
  005  ws-upgrade-path      draft      6 iterations  blocked on 004
  004  api-client-rewrite   draft      9 iterations  blocked — waiting on the API
                                                     contract (1d ago)
  003  settings-panel       ready      12 iterations approved 2h ago
  002  esm-migration        running    6 of 11       i-3ac8d0, 41m, 2nd attempt
  001  test-harness         done       4 of 4        2026-08-11
```

Every word in the status column is folded from each spec's own state block when
you ask. Nothing is stored. So it reads correctly on a machine that has never
executed anything, and `002` shows `running` on the laptop running it and
`ready` everywhere else. Both are true where they are.

One row is a state a stored field gets wrong.

```
  003  settings-panel       draft    12 iterations  approved at v3, now v4 —
                                                    re-approve (ratchet review 003 --changes)
```

You approved it, then edited an iteration.

#### Running it

```
$ ratchet run 001 --executor glm-4.7-flash@block
```

Then you leave. What you get while you are gone is short notifications on
whatever channel you configured. Not a log. A log is something you have to go
and read.

> **Iteration 6/11 · PASS**\
> 2 attempts · 134 min · 145 operations\
> `npm test`: 31 passing

> **Iteration 10/11 · needs you**\
> All 7 command gates pass. Waiting on acks: `reviewed`, `manually-tested`.\
> `ratchet ack i-b41c07 --reviewed` to release it.

> **Iteration 7/11 · BLOCKED**\
> The executor stopped and said why:\
> _"text.js's context argument changes the signature used by server.js, which is
> not in my files list. I cannot make `npm test` pass without editing it."_\
> `ratchet inspect i-7767cf` for the diff it did produce.

Short and separate, one idea each, so the first useful sentence is visible
without opening anything. Nothing is ever silently pending. A dead unattended
process and a working one look identical from outside, and the cost of that
ambiguity is that you stop trusting the system enough to leave.

A block is an outcome, not an error. The executor above is right and the spec
was wrong. The reason is the message rather than a link to it.

When you come back:

```
$ ratchet status 001

  001-esm-migration                                  10 of 11 · 1 waiting

  ✓  i-5b58a3  toolchain swap in package.json       1 attempt   ·   2m
  ✓  i-721439  check-tests guard script             1 attempt   ·   1m
  ✓  i-d5287f  first tests — network schema         1 attempt   ·   1m
  ✓  i-f09308  smoke harness and AGENTS.md          1 attempt   ·   2m
  ✓  i-7767cf  break the import cycle — world.js    2 attempts  ·  77m
  ✓  i-9e2b14  game.js reads world.js               2 attempts  · 135m
  ✓  i-3ac8d0  text.js takes a context argument     2 attempts  ·  95m
  ✓  i-8f1e6b  local.js migrated to native ESM      1 attempt   ·   8m
  ✓  i-2d47aa  spacerocks server module and index   1 attempt   ·   4m
  ⏸  i-b41c07  entry points and the esbuild bundle  commands green · needs ack
  ·  i-c60f92  documentation                        not started

  Every command gate from every completed iteration replays green against HEAD.
```

That last line is not decoration. Ratchet re-runs the accumulated gate set
against the current tree, so "iteration 3 passed" is a claim about now rather
than about a Tuesday. Iteration 8 breaking iteration 3 is the failure an
incremental system invites and the one a per-iteration score cannot see.

You review iteration 10's diff, run the app, and release it.

```
$ ratchet ack i-b41c07 --reviewed --manually-tested
  ✓ acks recorded · iteration 10 closed · resuming at 11
```

The executor never sees the ack verb, so there is no path by which the ack came
from anywhere else.

### From the drafter's seat

The drafter is a large model doing a job that looks nothing like coding. Its
instructions open with a sentence it needs, because every instinct runs the
other way: you are writing a document, not doing the work. A drafter that starts
editing files has failed, however good the edits are.

Five passes, and it is not alone for any of them.

**Read for structure, once per repo.** The drafter maps the codebase: module
graph, cycles, entry points, build, tests, landmines. Real thinking happens
here, and it may spend heavily, because tokens spent understanding the repo
become an iteration the executor can finish.

It is also the most expensive pass and identical work on the second spec for the
same repo. So it is not re-derived per spec. It is also not a committed
artifact. It is an index.

```
$ ratchet index --status

  /home/nathan/src/spacerocks   (tree 8f3c1ab, cached 2m ago)
  34 modules · 1 cycle · 2 entry points
  build: esbuild via `npm run build`   tests: node --test
  ⚠ cycle: game.js → entities.js → game.js

  lsp: typescript-language-server (attached)
```

Built on demand, cached by tree hash, never watched. A watcher would spend a run
invalidating an index whose consumer is idle, and cross-boundary filesystem
events from a bind-mounted container are unreliable on macOS. Building on query
cannot serve a stale answer.

Not committed and not reviewed. A `MAP.md` in the repo conflicts on every branch
that touches code, and the only correct resolution for a derived file is to
regenerate it. Reviewing a spec means checking decisions; reviewing a map means
spot-checking a parser, which you do once, to the parser. Provenance survives
more cheaply: a spec records the tree hash its index came from, and the journal
records the facts the drafter queried.

**What an index cannot hold** qualifies the case for having one. Any description
of a codebase mixes two kinds of entry.

```
34 modules · 1 cycle · 2 entry points                          ← derivable
⚠ src/text.js reads globals set in local.js — order-dependent  ← judgment
```

No language server will tell you a module depends on initialization order
established elsewhere. That is the expensive output of the structure pass. The
graph is a 200ms computation needing no model at all. An index does not save the
expensive work. It saves the cheap work.

Judgments get re-derived per spec, and mostly should. One that matters to a
single iteration belongs in that iteration. One that outlives specs is a
convention and earns a line in `AGENTS.md`, written or ratified by a person. So
the index holds what is derivable from code and `AGENTS.md` holds what is not.
What this rules out is a third file of machine judgments, committed and never
quite reviewed, going stale plausibly and specifically.

Queryable rather than a document buys two things. The executor can ask it
questions: `who imports src/util.js` is a 40-token answer where a dump is 6k.
And real language servers beat reimplementing a module graph, which upgrades the
pre-apply gate from "the file still parses" to unresolved imports, undefined
symbols and find-references. A syntax check catches a broken brace. A language
server catches the import you renamed in one place out of four, which is what a
14-file ESM migration is made of.

**Decide the seams.** Iterations are cut where the codebase already wants to
break. The constraint is unusual: not "what is a logical unit of work" but "what
can a model with 32k and no memory finish in one sitting". The second is
smaller. An iteration that touches four files and requires understanding six is
not one iteration.

**Size it.** The drafter estimates the input the executor will face: files to
read, design notes to carry, tool output it will generate.

```
$ ratchet size specs/001-esm-migration.md --context-window 32k

  iter  files  bytes   est. prompt   headroom
   5      3     18k       11.2k        66%
   6      2     31k       21.0k        36%   ⚠
   7      4     12k        9.8k        70%
```

Headroom below 40% is a warning. The executor needs room for the thing not in
this table: its own tool output. A build log is four thousand tokens. A failing
test run is more. An estimate that counts only the prompt fails on attempt two.

**Write the decisions down, repeatedly.** The last pass walks the iterations and
asks of each: if this were the only thing I read, could I do it correctly? Where
the answer is no, the missing decision gets copied in. This most distinguishes a
good spec from a bad one and feels most like waste while doing it.

**Ask, choose, or decide.** Those passes are a stream of decisions, and sorting
each one determines whether working with the drafter is pleasant.

| The decision is                        | It does              | You see                            |
| -------------------------------------- | -------------------- | ---------------------------------- |
| derivable from the code                | decides, silently    | nothing                            |
| a preference with a defensible default | decides, and says so | a recorded decision, not a prompt |
| yours, and it cannot proceed           | asks, and blocks     | a blocking question                |
| a fork with real costs on both sides   | writes a choice      | a comparison and a recommendation  |
| only answerable by looking at it       | builds a mockup      | rendered variants to click through |

The instruction is blunt, because the failure modes sit on opposite sides. An
drafter that asks about everything pushes the work back onto you, which is what
this product exists to stop. A drafter that asks nothing produces a confident
spec built on a guess, and you find out six hours in. When in doubt, decide and
say so. A recorded decision you can overturn in one word is cheap. Spend
questions where they change several iterations, not where they change a variable
name.

**Revise against threads, not a fresh prompt.** The drafter reads the threads,
revises the affected iterations, re-runs sizing and the mutation sweep, and
replies in each thread saying what it did or why it disagreed. Disagreeing is
expected. A comment that breaks the dependency chain should come back as a reply
explaining that, not a spec that silently complies and fails later.

The drafter can run commands, and it should. You do not learn what a build does by reading
it, and a drafter that cannot run the test suite is guessing about the thing it is sizing.
What it cannot do is write source: its repo is mounted read-only, and the only things it
writes are the spec and its mockups. So it investigates as freely as you would and changes
nothing you did not approve.

It never decides it has finished either. It hands back a draft with its open threads listed.
The spec is the only artifact a human is required to read closely, because everything
downstream of it is unattended.

One number is worth watching across specs. How often the executor comes back
`BLOCKED`. A question while drafting is cheap. The same question mid-run is a
spec defect.

### From the executor's seat

The executor is a local model. It wakes up knowing nothing.

Its prompt is one iteration: the goal, the design notes, the files, the steps,
and the command gates it must make pass. Not the spec. Not the previous
iteration. Not a summary of the project. One iteration and a clean tree at the
last commit.

The ack gates are not in there at all. No version of this prompt puts an
assertion only a human can make among the things the executor is told to
satisfy.

| Tool                             | Note                                                   |
| -------------------------------- | ------------------------------------------------------ |
| `read(path)`                     | scoped to the repo; returns a tagged, numbered listing |
| `edit(path, tag, line, end?, text)` | file tag plus line numbers, not exact-match strings |
| `write(path, text)`              | whole file; only for files in the iteration's list     |
| `bash(cmd)`                      | defanged environment; no pagers, progress bars, color |
| `revert_file(path)`              | restores one file to its iteration-start state         |
| `done(summary)`                  | claims the iteration is complete                       |
| `blocked(reason)`                | stops, and says why                                    |

**A read is bound to the file it came from, and lines are addressed by number.**
The read carries one short hash of the whole file; the lines themselves are bare.

```
[src/spacerocks/game.js#1A2B]
 8:function hello() {
 9:  console.log("world");
10:}
```

An edit quotes that header, names the lines, and gives both the line it expects
to find and the line it wants:

```
[src/spacerocks/game.js#1A2B]
PUT 9.=9:
-  console.log("world");
+  console.log("hello");
```

There is no whole-file `oldText`, so the most common failure we watched cannot
occur: an exact-match edit that stops matching because something reformatted the
file. The `-` line is scoped to the line already named, so it is a check rather
than a search — and checking it is what stops an edit that names the wrong line
from rewriting it. Measured across five local models, that check costs one
correct edit in 150 and takes edits landing in the wrong place from 24 to zero.

A bare line number without the header would be cheaper still, and worse: applied
to a shifted file it edits the wrong line silently. The header is what prevents
that. If the file has moved since the read, its hash no longer matches, and the
edit is refused rather than applied to whatever now sits at line 9. No silent
relocation, no fuzzy matching.

The design is `oh-my-pi`'s, and the property worth stealing is that the check is
free. Measured on four Q4 27-30B models against the same edits without the
header: **83 of 90 correct either way.** The tag is copied correctly essentially
always — one error in 210 attempts — because there is one of them per file rather
than one per line. Safety that costs accuracy gets removed the first time it is
inconvenient; this costs none.

When the tag does not match, the refusal depends on why, and Ratchet knows why
because it recorded what it handed out.

```
[E_ANCHOR_MISMATCH] This file changed after you read it. Line 50 is no longer
what you saw. Re-read before editing.
  49: for (const e of world.entities) {
  50:   if (!e.alive) continue;
  51:   e.update(dt);
```

**The refusal never hands back a tag to copy.** It shows what is there now and
asks for a fresh read, because the alternative is telling a model to edit content
it has not seen. Tested against four models: they re-read, on 151 of 154
occasions. On the other three they copied an identifier out of the error and
retried, and two of those would have deleted the wrong line — a constructor's
`super(id);` in one case.

So Ratchet adds a rule that wording cannot enforce: **an edit is refused unless
its tag was issued by a read in this session**, however exactly it matches the
file on disk. Matching proves the model has the right string, not that it read
the file. Method and limits in
the edit-interface measurements in the research notes.

**One edit per turn.** No batch form, so a partially applied multi-edit is not a
state the system can reach.

**Every edit is validated before it is applied.** The file is syntax-checked and
the result compared against the errors already there, so the executor sees only
problems its change introduced. A failed edit is not applied, and the executor
gets back three things: what went wrong, what its edit would have produced, and
what the file currently contains. All three are necessary. Without the error it
misdiagnoses. Without its own attempt it reissues the same edit. Without the
current content it edits against a memory from four turns ago.

**There is no `git`.** Not restricted. Absent. Two models answered a failed edit
by running `git checkout` over their own correct work, one across the whole
tree. The escalation is always the same shape: narrow action fails, reach for a
wider one. `revert_file` gives the intent somewhere safe to go.

**Nothing returns empty.** A command with no output comes back as "ran
successfully, produced no output". An executor handed silence starts running
things to find out what happened. We watched one burn 201 tool calls echoing its
own status at itself.

**There are exactly two ways to stop**, and both are named in every failure
message the executor sees. `done` and `blocked`. A model with no way to stop
will invent one, and what it invents will be worse.

A whole iteration from the executor's side:

```
→ read   src/game.js
→ read   src/world.js
→ read   test/game.test.js
·  The cycle is game → entities → game. world.js already exports the entity
   list, so game.js can import it directly and drop its re-export block.
→ edit   src/game.js  replace 1#QV..14#MT
→ edit   src/game.js  replace 402#SN..418#WB
→ write  test/game.test.js
→ bash   npm test
  ⤶ 31 passing
→ bash   node scripts/check-tests.mjs 23
  ⤶ ok
·  DONE: game.js now imports entities from world.js; removed the re-export
   block; added coverage for spawn and collision.
```

Sixteen operations. It stopped when it was done, because stopping was as easy as
continuing.

## What the runner guarantees

The runner sits between the seats and is the product. No outcome is silent, no
state unrecoverable, no number in a report a lie.

### Failures have types

| Type         | Means                                 | Response                                  |
| ------------ | ------------------------------------- | ----------------------------------------- |
| `transport`  | socket, timeout, 5xx from the runtime | retry, no attempt consumed                |
| `protocol`   | truncated or unparseable output       | retry with the stream repaired            |
| `capability` | the model did the wrong thing         | retry with the gate's output              |
| `budget`     | out of context, out of operations     | escalate; do not retry into the same wall |

`FAILED after 3 attempts` whether a socket dropped or the code was wrong is a
run you cannot learn from.

### Malformed output is a result, not a dropped turn

Output that cannot be parsed into a tool call comes back to the executor as a
tool result containing the error. It sees its own mistake next turn. It is never
told nothing happened.

Repairable output gets repaired: an unbalanced brace, a missing closing tag.
Truncated output does not, because closing an unterminated string produces a
valid-looking wrong value. Truncation is `protocol`, and the retry gets a
smaller context.

### Budgets are operations, not clock

A wall-clock timeout is a proxy for "stuck" and a bad one. It varies with
hardware and cannot tell a productive long attempt from a hung one. We killed an
iteration that was converging. Given three times the clock it passed.

Three independent dimensions. **Operations**, a hard count of tool calls per
attempt, deterministic and portable. **Idle**, time since the last event rather
than since the attempt began, which is the one thing a clock is good for:
detecting a dead transport. And **repetition**, keyed on tool plus normalized
arguments, with a warn state before the halt state and a counter that resets on
a different mistake.

The third exists because no time-based measure catches a loop. The 201-echo run
had the shortest gaps of anything we measured, median 0.7 seconds. It was
maximally busy.

The threshold comes from data. Legitimate work reached ten consecutive calls to
the same tool. The pathological loop reached 183. A guard set at eight, a real
default in a real framework, would have killed every passing run we have.

### Everything is recoverable

The tree is committed before an iteration begins and after it passes, so there
is no window where correct uncommitted work can be destroyed. A diff is
snapshotted after every operation, so a process that dies mid-iteration leaves
the work behind. `ratchet resume 001` picks up on any machine.

Over this research a laptop slept, a serving runtime was restarted by accident,
a home network dropped a host, and the harness was rewritten three times. None
of it cost any completed work.

### Undoing work

Three cases, and they are not the same case.

Reverting a whole iteration commit needs no reconciliation. The state row and
the code are one commit, so `git revert` removes both. The spec reports one
fewer iteration, the tree has one fewer in it, and `resume` restarts at the
missing one.

Reverting the code but not the record is caught by replay. A partial revert, a
hand-edited history, a botched cherry-pick. Ratchet does not believe the row; it
re-runs the accumulated gates against `HEAD`.

```
$ ratchet status 002

  ⚠ record and tree disagree
      i-9e2b14  closed 2026-08-16, gates now FAIL against HEAD
      3 later iterations depend on it

  the spec says 11 of 11 · the tree supports 5
  → ratchet reopen i-9e2b14   (appends a row; does not rewrite one)
```

Ratchet corrects forward. It never deletes the `closed` row, because an
append-only record you are willing to rewrite is a mutable field with extra
steps. It appends `reopened` with the reason, and the fold accounts for both.

Resume trusts the gates, not the record. Where to restart is the highest `N` for
which every iteration up to `N` currently passes. Those agree in the ordinary
case and diverge when something has gone wrong, and then the tree is the
authority.

A revert underneath a live run is a race and is reported as one. The runner
notes `HEAD` when an iteration begins. If `HEAD` moves and the runner did not
move it, the attempt is abandoned and restarted from wherever the tree now is,
logged as an external change. We have been on the wrong side of this.
Misattributing it costs hours debugging a model that was behaving perfectly.

### The executor runs in a container

The repo is bind-mounted and nothing else of yours is reachable. Not hardening
theatre. We watched what an executor reaches for when it is stuck.

Handed a gate it could not satisfy, one wrote a file of instructions for adding
an alias to the user's `~/.bashrc`. Not malice. That was the easiest remaining
way to make a command called `reviewed` exist. But the reasoning had a clear
line of sight from "my gate fails" to "modify the human's shell configuration",
and on an uncontained host it had the means.

Blast radius is one repo: no dotfiles, no `~/.ssh`, no other checkouts, no host
caches to poison. `git` is absent by construction, since the runner holds git
outside the container and commits freely while the executor has no path to
`checkout` or `reset --hard`. Policy is the weaker mechanism every time.
Defanging becomes reproducible: pinned toolchain, `PAGER=cat`, no color, npm's
fund and audit output off. On a shared host those drift; in an image they are
the image. And egress can be constrained to the registry and the model host,
where today an executor has the whole internet.

Two caveats. The model is not in the container. It stays on the host and the
container reaches it over the network, so what is contained is the agent
process, which is the part that touches your files. And on macOS, bind-mounted
I/O is slow enough that `npm install` notices. Over six hours, nobody notices.

### Never retry without changing something

A retry that re-sends an identical request is a wasted attempt. Every retry
carries the failing gate's output, an instruction naming what already succeeded
so it is not redone, and where the failure was `budget`, a smaller context.
Cline excludes `length` from retryable finish reasons by name, because retrying
re-hits it.

### Every address is generated, typed, and checkable

Two interfaces hand a model an address and ask it to act: line anchors for
editing files, iteration ids for closing work.

An address must not be descriptive. "The line that says `exports.entities`",
"the third todo", "iteration 10". Each can be wrong in a way the system cannot
detect. Two lines say `exports.entities`, todos get reordered, splitting an
iteration renumbers everything after it. So addresses are generated by the
system, typed so one kind cannot be mistaken for another, and checked at the
point of use.

So an iteration is `i-5b58a3` and a step inside it is `t-481af5`. The prefix is
a type, so handing `t-481af5` to something expecting an iteration is a loud
error rather than a wrong lookup. Stronger than separating two kinds of thing
into two schema fields, because it survives being passed through the wrong
field. Field separation protects a parser. A typed value protects everything
downstream of the parser.

The body is opaque and sparse. Six hex characters is sixteen million addresses
against a few dozen real ones, so a mistranscribed character lands on a
non-existent id and errors. The danger with opaque ids is never the invalid one.
It is the slip that lands on a different real id, and sparsity is what makes
that unlikely. Check it deliberately rather than assuming. Hex also dodges the
transcription hazards for free: no `o` to confuse with `0`, no `l` or `I` with
`1`. An id containing both `O` and `0` is the mistake.

Do not unify the two kinds. A line anchor addresses a mutable buffer and must
invalidate when content changes. An iteration id addresses a stable spec item
and must survive the spec being edited. Content-hashing an iteration id would be
a mistake. The shared principle is "generated, typed, checked", not "hash
everything".

The cost is that a model cannot trust a remembered address, so it lists ids
before acting. That is the trade this system asks for everywhere: pay operations
to turn a silent wrong action into a loud read.

It is also advice frontier models argue against. Asked whether ids should be
generated or caller-specified, they prefer caller-specified, and for a frontier
model that is correct: it holds a stable picture of a spec across a long
context, so ergonomics dominate. A model with 32k and no memory has the opposite
balance. This is a pattern. SWE-agent's current defaults disable a lint gate
worth three points on GPT-4 Turbo. Qwen Code's compaction constants go
degenerate below a 64k window. smolagents' documented answer to model
unreliability is a section titled "Use a stronger LLM." The defaults of this
ecosystem, and the advice of the models in it, are tuned for a model class we
are not using.

### The measurement is checked before the result

The rule that cost us the most, so Ratchet enforces it mechanically.

```
$ ratchet verify 001

  Command gates
    ✓ 11 blocks · 54 entries · every entry claimed exactly once
        52 → required_commands      2 → required_acks
    ✓ every command gate resolves to a program that exists
    ✓ gate sweep — 11 iterations reverted in throwaway worktrees
        every iteration has ≥1 gate that fails without its work, substantively
        4 gates unsweepable (fail in a bare worktree; see worktree_setup)
        note: 6 gates are invariants and pass either way, correctly

  Executor
    ✓ model resolved: glm-4.7-flash  (exact match, not a prefix)
    ✓ context allocated: 49152        (read from the runtime, not requested)
    ✓ compaction: fires at 39936, reclaims 24576
    ✓ session id unique to this run

  State
    ✓ 14 rows append-only since 2026-08-16 · 0 rows by: executor
    ✓ `ready` row hash matches the current spec (v4)

  Index
    ✓ derived from tree 8f3c1ab · 34 modules · lsp attached

  Sandbox
    ✓ podman 5.4.0 rootless · image ratchet-exec:node22 · git absent
    ⚠ egress: proxy only · iptables filtering unavailable rootless

  ✓ ready
```

Every line in the first two blocks is a trap we fell into. A model id that
prefix-matched something else, so the benchmark measured a model the report did
not name. A session id reused between runs, restoring the previous run's model
and its poisoned history. Compaction constants copied from a 48k configuration
onto a 32k window, reclaiming four thousand tokens of thirty-two. A gate that
had never been observed to fail, and could not.

**The first check is a reconciliation.** The tempting version counts what went
wrong: "no acks were parsed as commands". That is worthless, because a parser
that read nothing satisfies it and goes green. So the assertion is that the
parse is total. 54 entries, all 54 claimed by exactly one field, each in the
field matching its section. It fails on a misclassified ack, on a silently
dropped command, and on a parser returning nothing. The per-spec check is the
second line of defense; the first is a unit test on the parser, watched to fail
before it was trusted.

**The exit-127 clause closes a hole.** "This gate fails against a broken tree"
sounds sufficient. It is not: `reviewed` fails against a broken tree too,
because everything fails when the program does not exist. So the failure must be
substantive. A test that reports failures, a compiler that reports errors, an
exit code that is not `command not found`. This also catches a person typing
`reviewed` under `required_commands` by hand, which parses perfectly and is
still wrong.

**The gate sweep asks whether an iteration's gates notice its work missing.**
Revert the files the iteration declares, run its gates, and see what happens. If
they all pass, a model could have closed that iteration having done nothing, and
every gate would have been green.

The assertion is over the iteration, not over each gate, because gates come in
two kinds. A **progress** gate asserts the work happened and must fail on a
revert. An **invariant** gate asserts something did _not_ change —
`git diff --quiet -- scripts/check-tests.mjs` says a guard file was left alone —
and a well-behaved executor leaves it alone either way. Requiring that to fail
would be requiring the wrong thing. What matters is that at least one gate in the
iteration is a progress gate.

Reverting is the only perturbation that needs to know nothing about the project.
The iteration already declares its files, so the check is derived from the spec
rather than from any guess about language or framework. Mutations aimed at what a
gate _checks_ are as project-specific as the gate, and a library of those per
language is years of work Ratchet does not do.

It runs in a throwaway `git worktree`, never the real tree, because
recoverability is not suspended for the benefit of

Two assertions fall out. Every gate must fail under at least one mutation,
because a gate sensitive to nothing is `true` in a costume. And no gate may fail
under all of them, because a gate objecting to mutations with no bearing on it
fails unconditionally. That is `reviewed`'s shape, and the pattern an exit-code
check alone misses on a machine where `reviewed` happens to exist. Ratchet
reports the weakest gate rather than a tick.

The limit: this proves sensitivity, never sufficiency. An iteration whose only
gate is `node --check` fails the syntax mutation and sails through a
semantically wrong migration. Ratchet catches a gate that checks nothing, not
one that checks too little. That judgment is a concrete reason the spec is a
document a person reads. The sweep is budgeted and reports which gates went
unswept, because a bounded check presenting as a complete one is worse than
none.

Two more guarantees. Ratchet never reads a working tree or a log being written.
And every constant is derived from the thing it depends on: compaction from the
real window, observation caps from the real context, idle allowances from
measured host throughput. A constant correct on one machine is a silent bug on
the next.

`ratchet verify` runs automatically before every `ratchet run`. Not a command
you have to remember, because the whole category of bug it prevents is made of
things nobody remembered.

## Interfaces

The CLI is where you start and control everything. It is small enough to print
in full, which is the shape it is meant to have.

```
$ ratchet --help

  Ratchet — run a local model as an unattended software engineer.

  Drafting                                      (a human is present)
    plan [intent]         open a session; draft a spec. Opens the page.
                            no intent given → state it in the page
    review <spec>         re-attach to a spec's session page
      --changes             print the diff since you last looked
    size <spec>           per-iteration context estimate and headroom
      --context-window N    size against a window other than the spec's own
    ready <spec>          approve for running; refuses while anything is open

  Running                                       (nobody is present)
    verify <spec>         gate, executor, state, index and sandbox pre-flight
                            runs automatically before `run`; this is for looking
    run <spec>            execute iterations until done, blocked, or out of budget
      --executor M@HOST     which local model, on which machine
      --from <iteration>    start later than the first unfinished one
    resume <spec>         continue where the tree says it got to
    stop <spec>           end the current attempt; commit nothing in flight

  Looking                                       (cheap, read-only, offline)
    list                  every spec and its folded status
    status <spec>         per-iteration record; replays gates against HEAD
    inspect <iteration>   the diff, the operations, the classified failures
    log <spec>            the state block, rendered

  Deciding                                      (human verbs; no agent has these)
    ack <iteration> --<name>...   supply an ack gate
    block <spec> "reason"         record a human-asserted blocker
    unblock <spec>                resolve it
    reopen <iteration>            record that closed work no longer holds

  Environment
    index [--status]      build or report the repo index
    doctor                check hosts, models, allocated context, sandbox image
    qualify <model>       run the model against Ratchet's own interfaces and
                          record what it can do

  Global
    --json                machine-readable output, for every subcommand
    --quiet               errors only
```

Three things about that surface are deliberate. The four groups are the phases:
drafting commands assume somebody is there, running commands assume nobody is,
and `Deciding` is exactly the set of verbs no agent possesses. Nothing mutates a
spec's meaning, so there is no `ratchet edit`, no `set-status`, no way to mark
an iteration done. And every subcommand takes `--json`, because a tool whose
output is only human-readable is something you cannot build on.

The session page is the second surface, and for the whole drafting phase the
first. `ratchet plan` opens it before there is anything on it. Until approval it
is where the intent is stated, progress watched, questions answered, mockups
picked and the draft marked up. `ratchet review <id>` attaches again and never
creates a second one. It stops at approval. There is no run dashboard, because
there is no human in that half of the system for a live page to be live for.

Notifications are the third, and for unattended work the primary one. Short
messages, one idea each, on iteration boundaries and on anything needing you.
Voice out for when you cannot read, voice in for when you cannot type. The
moments you most want to check on an unattended run are the moments you are not
at a keyboard.

The journal is the fourth. A state block is a dozen committed rows, so
`ratchet list` works on a fresh clone; the journal is everything else about one
run, on the machine that ran it. `runs/001/journal.jsonl` holds every operation,
gate result, classified failure and ack with its timestamp, and from drafting,
every question, decision, choice and mockup with the variant picked.

Both are append-only and `ratchet status` is computed from them. Nothing in
Ratchet's reporting is stored separately from its evidence. There is no field to
have drifted.

## Qualifying a model

`doctor` asks whether the machine is fit. `qualify` asks whether the **model** is
fit, and they are different questions with different answers.

```
$ ratchet qualify glm-4.7-flash:32k

  addressing     30/30   names a line from a tagged read
  edit body      7/30    ⚠ damages leading whitespace on 22 of 30
  sigils         0/30    ✗ drops line-initial + and - markers
  substitution   30/30   with the header form; 1/30 with the diff form
  staleness      30/30   re-reads when the file moved under it
  terminal       30/30   stops on done, does not invent verbs

  qualified, with adjustments:
    edit syntax      header form        (diff form unusable: 0/30)
    read window      400 lines
    retry            on refusal         (recovers 45% of diagnosed failures)

  Written to .ratchet/models/glm-4.7-flash-32k.json
```

**The output is a configuration, not a grade.** A score tells you to pick a
different model; a profile tells Ratchet how to drive the one you have. That is
the escalation ladder pointed at ourselves: where a model has a weakness the tool
can work around deterministically, the tool works around it, and the human is
told rather than asked.

The thing that makes this necessary rather than tidy is that **you cannot tell a
good interface from a bad one by looking at it.** Five edit forms were measured
across five local model configurations. The spread between them is 78 of 150 to
147 of 150, and the worst was not obviously worse on paper — it differed from the
best only in where the payload boundary sat. One form scored 30 of 30 on a model
that managed 1 of 30 on another form the rest of the set handled comfortably.

The optimistic half is that a well-chosen form did win for everyone: the best
scored at least 28 of 30 on every model, with no edit landing in the wrong place.
So this is not an argument that every model needs its own interface. It is an
argument that which interface that is, and what it must check, is a measurement
rather than a judgment — and it moved by 69 points of 150 on wording
choices that all looked reasonable when written.

Nor does general capability predict it. The strongest model in the set was the
one most willing to skip a re-read after the file moved beneath it — capable
enough to think it could work out the answer, and wrong about half the time it
tried.

A profile is stale the moment anything under it moves, so it is keyed by model
tag, quantisation, the serving engine's version, and the version of Ratchet's own
prompts. Any of those changing invalidates it, because all of them have changed a
result at least once.

Failing does not block anything. `run` will use an unqualified model and say so;
what it will not do is silently pick settings for a model it has never tested.

## What Ratchet does not do

**It does not judge its own work with a model.** Attempt scoring by an LLM judge
exists in mature frameworks and we do not use it. A judge wrong half the time
makes three attempts worse than one. Ratchet's scorers are deterministic: the
gates pass, the tests pass, the diff is non-empty. If that cannot express your
definition of done, the missing piece is an ack gate.

**It does not fuzzy-match edits.** Whitespace and Unicode normalization, yes: a
quantised model retyping code produces curly quotes and en-dashes, and that is a
transcription artifact. Similarity matching, no. Every mature editor we examined
either avoids it or has disabled it in place. An edit applied to code that
merely resembles the target is a corruption that passes review.

**It does not summarize its own uncertainty away.** A blocked iteration gives
you the executor's own sentence. A stopped run gives you which of the four
failure types stopped it. Ratchet would rather report a confusing truth than a
clean number.

**It does not require a large model to execute.** That is the point. It requires
one to drafter, once, in the open, in a document you read. Then the work happens
on hardware you own.

## The premise, restated

There is no deterministic explanation to find. Sampling is stochastic. Serving
runtimes have open bugs they have had for months. Networks drop, laptops sleep,
and constants calibrated on one machine are silently wrong on the next. Every
attempt this project made to explain a class of failure completely ended in a
mechanism covering one case in four.

So Ratchet does not try. It assumes the model is roughly good enough, which is
now a measured claim rather than a hope, and spends its design budget on three
properties. Failures are visible: typed, reported by kind, never silently
absorbed. States are recoverable: committed after every increment, snapshotted
between them, destroyed only by an action a human took on purpose. Numbers are
honest: every report computed from evidence stored beside it, every gate watched
to fail before it was trusted to pass.

The goal was never to explain every failure. It was to build so failure is
survivable, and so that when it happens, you find out.
````
