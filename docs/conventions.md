# Conventions

How code in this repo is written. Each entry says whether it is enforced or left
to judgement, because the difference decides how much it can be relied on.

A convention that is only written down gets violated. That is measured here, not
assumed: in the harness that preceded this repo, a trap was recorded in a
comment on the very field that carried it, and the same defect shipped a day
later, costing one model 0 of 50 on a probe. So where a rule can be checked
mechanically, it is, and this file says which.

## Writing

Applies to documentation, code comments, commit messages and pull request
bodies.

**Get to the point.** Never use two words where one will do.

**Avoid jargon.** Use a technical term when it is the right term. Do not
compress meaning into an idiom the reader has to unpack.

**Assume the reader is smart and does not know the problem.** Explain the
problem, not the vocabulary.

**Use simple language and an imperative tone.**

**Never write in the first or second person, and never name a person.** No "I",
"we", "us", "you", "your", or anyone's name, in documentation, comments, commit
messages or pull request bodies. Write about the code and the problem.
"Measured, the per-line scheme cost 15 answers in 30" outlives "we measured",
which dates the text and ties it to whoever was in the room.

Naming the audience in the abstract is fine: "a reader has the diff already" is
about the code's audience, not about a person.

**Refine with the `avoid-ai-writing` skill** when it is available. It catches em
dashes, bold that carries no weight, and the phrasings that read as machine
output.

## Go

**Enforced by `make lint`.** `gofmt`, `go vet`, `staticcheck`. No exceptions and
no `nolint` comments; if staticcheck is wrong, say why in the code.

**Enforced by `internal/convention`.** Every exported type, function, method and
struct field carries a doc comment starting with its name. A visual pass over
the predecessor harness missed twenty of these.

**Enforced by `internal/convention`.** A declared function opens and closes its
braces on different lines. A one-line body reads as a value rather than as code,
so the next person adds a statement and reformats the whole thing, and the diff
hides what changed. Function literals are exempt: a small transform passed as an
argument is the one place the compact form is clearer.

**Judgement.** A doc comment sits on the thing it describes. `xxHash32`'s
comment was attached to the const block above it, where a reader looking at the
function never sees it.

**Judgement.** Wrap a call chain after the dot, with the chain indented, rather
than by breaking the argument list:

```go
g.Expect(Tag(changed)).
    NotTo(Equal(s.Tag), "the file moved and the tag must say so")
```

Breaking after the opening paren separates a matcher from its message and reads
as two statements. Long lines are acceptable in tests; a long line in production
code usually means the expression wants a name.

**Judgement.** Comments say why, not what. A comment that restates the code is
noise; a comment naming the measurement or the incident behind a decision is the
only place that information survives. Prefer the incident: "this cost one model
49 of 50 silently" is checkable, "be careful here" is not.

**Judgement.** A set is `map[T]struct{}`, not `map[T]bool`. A bool implies that
`false` means something, and a reader has to work out whether an absent key and
a `false` value differ. `struct{}` has no value to misread, so membership is the
only question the type can answer.

**Judgement.** Vendor a dependency when it must agree byte for byte with someone
else's implementation forever and it is small. `internal/anchor`'s xxHash32 is
vendored for that reason and pinned against the reference library's own vectors.

## Tests

**Enforced by `internal/convention`.** A table test runs each row in its own
`t.Run`, and creates its gomega instance **inside** the closure:

```go
for _, c := range cases {
    t.Run(c.name, func(t *testing.T) {
        g := NewWithT(t)
        g.Expect(Tag(c.in)).To(Equal(c.want))
    })
}
```

Binding gomega to the parent `t` attributes the failure to the function instead
of the row, hides the row's name, and stops the table at the first failure. The
closure fixes all three for one line.

**Enforced by `internal/convention`.** A table is a named variable, not a
literal inside the `range`, and its rows name their fields, one per line:

```go
vectors := []struct {
    name string
    in   string
    want string
}{
    {
        name: "empty",
        in:   "",
        want: "5D05",
    },
}

for _, v := range vectors {
    ...
}
```

An inline table puts the data between `range` and the loop body, so reading the
loop means reading past the whole table. Positional fields stop being readable
past two of them, and adding a field silently reassigns every existing value.

The struct type is declared **inline**, as above, rather than as a named type. A
table is common enough that a named row type costs a lookup and buys nothing,
and the fields are read right where the rows are. This is the one place an
anonymous struct is preferred to a declared one.

**Assertions use gomega**, in plain `go test` functions rather than under
ginkgo: `NewWithT(t)`, then `Expect(...).To(...)`. See the replay example in
[architecture.md](architecture.md).

**Judgement.** Pin against an independent source, not against another copy of
the same code. The tag vectors came from a sibling implementation in the same
project; a test pinning one port against another passes on a shared mistake, so
they were re-verified against the reference library.

**Judgement.** Prove each check can fail _individually_. `NewWithT` fails
fatally, so a second assertion in the same subtest never runs once the first has
failed: the positional-field check above passed its own review only after being
tested on its own, because the inline-table check beside it had masked it.

**Judgement.** Prove a new gate can fail before trusting a pass from it. Write
the violation, watch the gate go red, then remove it. Three checks in the
predecessor project could not have failed.

**Judgement.** Write that violation from the complaint, not from the
implementation. The subtest check above was proved with a gomega created inside
the loop, which it caught. The shape that prompted the rule was a gomega created
_above_ the loop and reused inside, and that passed the gate. A gate proved
against a violation of its author's choosing tests the author's reading of the
rule.

**Judgement.** Cover the adversarial shapes, not only the happy path. For
anything addressing lines: identical adjacent lines, a one-line file, a file
ending without a newline, a blank line, and content containing the characters an
address uses.

Combine them, because the bug lives in the combination. Each of those shapes was
covered individually and `MintAll` still lost a line, because the failing file
was two of them at once: no trailing newline _and_ a last line holding only
indentation. That file is routine in Python, and the lost line would have been
refused as never displayed.

## Commits

**Never commit or push without asking.** Approval of one commit is not approval
of the next. A change to a drafted message is not approval either: redraft, then
ask again.

**Format:**

```
<one sentence saying what the commit does>

<one or more paragraphs saying why, if a reason is needed>
```

**Subject.** One sentence, imperative, under about 72 characters.

**Body.** Why the change was made: the problem, the constraint, the tradeoff.
Not a restatement of the diff. A reader has the diff already and cannot recover
the reason from it.

Leading with the problem and then pivoting into what was built is still a
what-body. If a paragraph would survive being replaced by `git show`, cut it.

**No trailers.** No `Co-Authored-By`, no `Signed-off-by`.

**No references to specs, plans, tasks or issues,** unless the commit is that
thing. Commits are permanent and those references rot.

**State a correction as a correction.** If a commit reverses an earlier claim,
name the claim and say why it was wrong. A silent reversal leaves two
contradictory statements in the history and no way to tell which one won.

**Draft in phases.** Write it, re-read it, then hand it to a subagent running a
simple model such as Haiku and ask whether the body gives a reason or summarises
the diff. Rewrite if it summarises, send it back for a second review, then ask
for approval.

Use a subagent rather than reading it again yourself. The author cannot see that
a sentence is scaffolding, because the author knows what it was holding up.

**Ask with a choice, not an open question.** Present the drafted message through
a tool that offers the options: commit it, commit the subject line alone, do not
commit, or change something. An open question invites a yes that was meant as a
comment.

## Specs

**Judgement.** Carry the design in the spec, with the rejected alternative
named. An iteration that passes every size budget and omits the reasoning failed
eight times slower than its reference run, because the executor reinvented a
design the author had already rejected.

**Judgement.** One iteration does one kind of work, and gates on commands
wherever a command can prove it. Reserve an ack for what only a person can
assert.

**Enforced by `backlog`.** Iterations close in order, and an iteration's
`required_commands` must pass before it closes.
