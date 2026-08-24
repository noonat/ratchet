package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"

	"ratchet/internal/agent"
	"ratchet/internal/edit"
	"ratchet/internal/patch"
)

// The names the executor's calls are advertised and dispatched under. A literal
// in both places can drift in one of them.
const (
	// NameRead reads a file and records what it served.
	NameRead = "read"
	// NameEdit applies one patch.
	NameEdit = "edit"
	// NameDone stops, claiming the work is finished.
	NameDone = "done"
	// NameBlocked stops, because the work cannot be finished.
	NameBlocked = "blocked"
)

// Names lists the calls in the order they are advertised.
func Names() []string {
	return []string{NameRead, NameEdit, NameDone, NameBlocked}
}

// Tools is the executor's call surface over one session.
//
// Four calls of the seven the design names: enough to edit one file and stop.
type Tools struct {
	session *Session
}

// NewTools wraps a session in the calls the executor is given.
func NewTools(s *Session) *Tools {
	return &Tools{session: s}
}

// Tools advertises the calls, with the strict schema for each.
//
// Strict is what the model is shown. Execute accepts more, because a call whose
// intent is clear is worth taking and advertising the loose shape would teach it.
func (t *Tools) Tools() []agent.Tool {
	return []agent.Tool{
		{
			Name:        NameRead,
			Description: "Read a file. Returns a tagged header and numbered lines. An edit must cite a read.",
			Schema:      object(required("path", "string", "the file to read, relative to the repository root")),
		},
		{
			Name: NameEdit,
			Description: "Apply one patch, in the notation a read is written in: " +
				"a section header naming the path and the tag, then the hunks.",
			Schema: object(required("patch", "string", "the section header and hunks")),
		},
		{
			Name:        NameDone,
			Description: "Stop, claiming the iteration's work is finished. A claim, not a decision.",
			Schema:      object(required("summary", "string", "what was done")),
		},
		{
			Name:        NameBlocked,
			Description: "Stop, because the work cannot be finished. Say what is in the way.",
			Schema:      object(required("reason", "string", "what is in the way")),
		},
	}
}

// Execute runs one call and returns what the model is shown.
//
// A refusal comes back as the result, unedited. Its wording is measured work, so
// summarizing it would throw away the measured part.
func (t *Tools) Execute(ctx context.Context, call agent.ToolCall) (agent.Outcome, error) {
	switch call.Name {
	case NameRead:
		path, widened, err := text(call.Args, "path", "file_path", "file")
		if err != nil {
			return agent.Outcome{}, err
		}
		out, err := t.session.Read(path)
		if err != nil {
			return agent.Outcome{Text: err.Error(), Widened: widened}, nil
		}
		return agent.Outcome{Text: out, Widened: widened}, nil

	case NameEdit:
		body, widened, err := text(call.Args, "patch", "text", "edit", "content")
		if err != nil {
			return agent.Outcome{}, err
		}
		p, err := patch.Parse(body)
		if err != nil {
			return agent.Outcome{Text: err.Error(), Widened: widened}, nil
		}
		res, err := t.session.Edit(ctx, *p, edit.Options{})
		if err != nil {
			return agent.Outcome{Text: err.Error(), Widened: widened}, nil
		}
		return agent.Outcome{
			Text:    fmt.Sprintf("applied. the file is now [%s#%s]\n%s", p.Path, res.Tag, res.Edit.Diff),
			Widened: widened,
		}, nil

	case NameDone:
		said, widened, err := text(call.Args, "summary", "message", "text")
		if err != nil {
			return agent.Outcome{}, err
		}
		return agent.Outcome{Text: "recorded", Stop: agent.StopDone, Said: said, Widened: widened}, nil

	case NameBlocked:
		said, widened, err := text(call.Args, "reason", "message", "text")
		if err != nil {
			return agent.Outcome{}, err
		}
		return agent.Outcome{Text: "recorded", Stop: agent.StopBlocked, Said: said, Widened: widened}, nil

	default:
		// An unknown name is an error, not a result: another turn cannot make the
		// tool exist. The message lists the real ones.
		return agent.Outcome{}, errors.Newf("no tool named %s; the tools are %s", call.Name, list(Names()))
	}
}

// text takes a string argument under the advertised name or a spelling a model
// reaches for instead, and says which it took.
func text(args map[string]any, want string, also ...string) (string, []string, error) {
	names := append([]string{want}, also...)
	for _, name := range names {
		v, ok := args[name]
		if !ok {
			continue
		}
		s, err := asString(v)
		if err != nil {
			return "", nil, errors.Wrapf(err, "the %s argument", name)
		}
		if name == want {
			return s, nil, nil
		}
		return s, []string{fmt.Sprintf("%s was given as %s", want, name)}, nil
	}
	return "", nil, errors.Newf("no %s argument; it is required", want)
}

// asString widens the shapes a string arrives as.
func asString(v any) (string, error) {
	switch got := v.(type) {
	case string:
		return got, nil
	case float64:
		return fmt.Sprintf("%v", got), nil
	case int:
		return fmt.Sprintf("%d", got), nil
	case []any:
		// One value in an array is a model using another tool's shape. Two is a
		// batch, which no schema here has a form for.
		if len(got) == 1 {
			return asString(got[0])
		}
		return "", errors.Newf("%d values where one was declared", len(got))
	}
	return "", errors.Newf("a %T where a string was declared", v)
}

// list renders names with an "and" before the last. The message goes to a model,
// which is still a reader.
func list(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// object wraps properties in the JSON Schema shape a tool advertises.
func object(props map[string]any, req ...string) map[string]any {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	if len(req) > 0 {
		names = req
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   names,
	}
}

// required declares one required string property.
func required(name, kind, description string) map[string]any {
	return map[string]any{
		name: map[string]any{"type": kind, "description": description},
	}
}

// compile-time proof that the executor's tools are a dispatcher the loop holds.
var _ agent.Dispatcher = (*Tools)(nil)
