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

**Judgement.** A struct literal that does not fit on one line puts every field
on its own line, keyed, with a trailing comma and the brace alone:

```go
return nil, &Fault{
    Line:   n,
    Reason: "a body row starts with `-` or `+`",
}
```

Not `&Fault{Line: n,` with the rest hanging below. The keyed form lets gofmt
align the values, adding a field touches one line instead of reflowing the
literal, and the closing brace shows where the value ends.

**Judgement.** Wrap a call chain after the dot, with the chain indented, rather
than by breaking the argument list:

```go
g.Expect(Tag(changed)).
    NotTo(Equal(s.Tag), "the file moved and the tag must say so")
```

Breaking after the opening paren separates a matcher from its message and reads
as two statements. Long lines are acceptable in tests; a long line in production
code usually means the expression wants a name.

**Judgement.** Do not wrap to hit a margin. Go tolerates long lines, and a
120-character call holding one argument and one string reads better whole than
split. Never break a string constant across lines for width: the reader then has
to reassemble the message to know what it says.

Wrap when a call is genuinely long, hundreds of characters or several arguments
worth reading separately. Then give each argument its own line, with a trailing
comma and the paren alone:

```go
return nil, faultAt(
    n,
    reason,
    hint,
)
```

The same shape as a struct literal, for the same reasons.

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

## Dependencies and errors

Moved here from the architecture document, which describes how the system is
shaped rather than how its code is written.

**Stdlib-first.** Git is shelled out to, not linked. The third-party list is
short and each entry is a decision with a reason.

| Dependency           | Why                                               |
| -------------------- | ------------------------------------------------- |
| `urfave/cli/v3`      | position-independent flags; confined to `main.go` |
| `goldmark`           | the spec parser is a real Markdown AST, not regex |
| `yaml.v3`            | the `ratchet` blocks                              |
| `cockroachdb/errors` | a stack at the site an error was constructed      |
| `esbuild` (Go API)   | the index's module graph; compiles the page's TS  |
| `htmx` (vendored JS) | fragment swaps and SSE; pinned by version, digest |
| `gomega` (test only) | assertions                                        |
| `tsc` (dev only)     | typechecking; the only thing that wants node      |

Position-independent flags are for agents, not people.
`ratchet ack i-b41c07 --reviewed` and `ratchet ack --reviewed i-b41c07` mean the
same thing, and argument order is the sort of thing a model gets wrong once per
session forever.

Package-qualified, de-stuttered names. `spec.Parse`, not `spec.ParseSpec`. The
exception is each package's namesake type, which keeps the name: `spec.Spec`,
`anchor.Anchor`, the `context.Context` idiom, reserved for the type that is the
package's reason to exist.

A stack at the earliest possible origination point, exactly once.

For a foreign error that point is the boundary where it enters this code: wrap
it on the first line, with `errors.Wrapf` when there is context worth adding and
`errors.WithStack` when the error already names its operation.
`internal/sandbox` is the reference: every engine `exec` failure carries the
command and its output.

For an error originating here, that point is where the return chain starts.
Attach the stack where the error is constructed, not where it is finally
handled, because by then the frames that would say which of eleven return points
produced it are gone.

Bare-return anything that already carries a stack, and re-wrap only to add
context.

`wrapcheck` stays off. It flags any error crossing a package boundary unwrapped,
which would force redundant wraps on errors that already have stacks.

**Judgement.** A `Fault` carries a stack, attached where it is constructed. An
earlier version of this file argued the opposite, on the grounds that the corpus
replay produces thousands of faults and a stack on each is noise. That confused
attaching a stack with printing one. The stack costs a slice of program counters
and renders only under `%+v`, while losing it costs the one thing that says
which of eleven return points produced the fault.

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
