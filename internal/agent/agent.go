// Package agent drives a model host: the provider interface, the streamed turn,
// and the shapes a turn is made of.
//
// The interface is deliberately two methods. A seat that swaps one host for
// another swaps an implementation, and the loop above never learns which it has.
package agent

import "context"

// Provider is a model host one turn can be sent to.
type Provider interface {
	// Stream sends a turn and returns the whole reply, calling on for each piece
	// as it arrives.
	Stream(ctx context.Context, req Request, on func(Event)) (Reply, error)
	// Allocated reports the context the host actually gave a model.
	Allocated(ctx context.Context, model string) (int, error)
}

// Role is who a message came from.
type Role string

const (
	// RoleSystem is the standing instruction.
	RoleSystem Role = "system"
	// RoleUser is the iteration and everything handed to the model.
	RoleUser Role = "user"
	// RoleAssistant is what the model said, including the calls it asked for.
	RoleAssistant Role = "assistant"
	// RoleTool is a call's result, going back as its own turn.
	RoleTool Role = "tool"
)

// Message is one entry in the conversation.
type Message struct {
	// Role is who it came from.
	Role Role
	// Content is the text.
	Content string
	// ToolCalls are the calls an assistant message asked for.
	ToolCalls []ToolCall
	// ToolName names the call a tool result answers.
	ToolName string
	// Thinking is the reasoning the reply carried, kept because a reply that
	// spent its whole budget on it arrives with empty text and is otherwise
	// indistinguishable from a model that answered nothing.
	Thinking string
	// Done is the host's reason for stopping, such as stop or length. A turn that
	// stopped at the cap is a different failure from a turn that answered badly,
	// and only this says which.
	Done string
}

// Tool is a call advertised to the model.
type Tool struct {
	// Name is what the model calls.
	Name string
	// Description says when to call it.
	Description string
	// Schema is the JSON Schema for the arguments, advertised strictly. What is
	// accepted back is wider, because a call whose intent is not in doubt is
	// worth taking; advertising the wider shape teaches the sloppiness instead.
	Schema map[string]any
}

// ToolCall is a call the model asked for.
type ToolCall struct {
	// Name is the tool it named.
	Name string
	// Args are the arguments as they arrived, before any widening.
	Args map[string]any
}

// Request is one turn's input.
type Request struct {
	// Model is the model to run.
	Model string
	// Messages are the conversation so far.
	Messages []Message
	// Tools are the calls the model may make.
	Tools []Tool
	// NumCtx is the context to ask the host for.
	NumCtx int
	// Predict caps the reply, with zero leaving it to the host.
	Predict int
	// Think is the reasoning setting, empty to leave the field off the request.
	// Off is not the same as absent: absent lets a reasoning model reason.
	Think string
}

// Event is a piece of a reply as it arrives. It exists so a caller can show
// progress and so the idle timer has something to reset on.
type Event struct {
	// Content is a piece of the reply text.
	Content string
	// Thinking is a piece of the reasoning, which newer hosts stream separately.
	Thinking string
}

// Reply is a finished turn.
type Reply struct {
	// Content is the reply text.
	Content string
	// Thinking is the reasoning, kept because a reply that spent its whole budget
	// on it arrives with empty text and is otherwise indistinguishable from a
	// model that answered nothing.
	Thinking string
	// ToolCalls are the calls the model asked for.
	ToolCalls []ToolCall
	// Done is the host's reason for stopping, such as stop or length.
	Done string
	// EvalTokens is what the reply cost to generate.
	EvalTokens int
	// PromptTokens is what the request cost.
	PromptTokens int
}
