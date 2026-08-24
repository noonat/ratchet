package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// DefaultIdleTimeout is how long a stream may produce nothing before it is
// abandoned.
//
// The timer resets per token rather than bounding the request, because a model
// thinking for four minutes and a dead socket look identical to a request-level
// deadline, and only one of them is worth waiting for.
const DefaultIdleTimeout = 90 * time.Second

// ollamaPort is the host's default port.
const ollamaPort = "11434"

// chunkBufferSize is the room given to one streamed line before it has to grow. A
// chunk is normally a few tokens; a tool call arrives whole and is larger.
const chunkBufferSize = 64 << 10

// maxChunkSize bounds one line. A stream sends the reply in pieces, so a line past
// this size is a host malfunctioning rather than a model answering at length,
// and reading it would be an unbounded allocation driven by a remote.
const maxChunkSize = 16 << 20

// Ollama drives a host over its native chat route.
//
// The native route is used rather than the OpenAI-compatible one because that
// route discards the context option without saying so: a model asked for 48k
// loads at 4096 and every reply looks fine. What was asked for is unrecoverable
// from the reply, so the request has to go where the option is honored.
type Ollama struct {
	// Addr is host:port.
	Addr string
	// IdleTimeout is how long the stream may produce nothing, defaulting to
	// DefaultIdleTimeout.
	IdleTimeout time.Duration
	// Client is the HTTP client, defaulting to one with no request timeout, since
	// the idle timer is what bounds a turn.
	Client *http.Client
}

// NewOllama returns a provider for one host, defaulting the port.
func NewOllama(addr string) *Ollama {
	return &Ollama{
		Addr:        withPort(addr, ollamaPort),
		IdleTimeout: DefaultIdleTimeout,
		Client:      &http.Client{},
	}
}

// withPort adds a port to an address that has none, and brackets a bare IPv6
// literal on the way.
//
// Looking for a colon is wrong for IPv6: `::1` is full of them and carries no
// port, so a bare address would be left alone and then produce an unparseable
// URL. SplitHostPort answers the actual question and JoinHostPort writes the
// brackets a URL needs.
func withPort(addr, port string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, port)
}

// Stream sends a turn and returns the whole reply, calling on for each piece.
func (o *Ollama) Stream(ctx context.Context, req Request, on func(Event)) (Reply, error) {
	body, err := json.Marshal(o.wire(req))
	if err != nil {
		return Reply{}, errors.Wrapf(err, "encoding the request")
	}

	// The idle timer cancels this context, so a stream that stops producing is
	// abandoned while one that is still producing is not.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	starved := false
	idle := time.AfterFunc(o.idle(), func() {
		starved = true
		cancel()
	})
	defer idle.Stop()

	url := "http://" + o.Addr + "/api/chat"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Reply{}, errors.Wrapf(err, "building the request to %s", url)
	}
	hreq.Header.Set("Content-Type", "application/json")

	res, err := o.client().Do(hreq)
	if err != nil {
		if starved {
			return Reply{}, errors.Newf("%s produced nothing for %s", req.Model, o.idle())
		}
		return Reply{}, errors.Wrapf(err, "calling %s", url)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Reply{}, errors.Newf("%s returned %s", url, res.Status)
	}

	var reply Reply
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, chunkBufferSize), maxChunkSize)
	for sc.Scan() {
		idle.Reset(o.idle())
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk chatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			return Reply{}, errors.Wrapf(err, "decoding a chunk from %s", req.Model)
		}
		if chunk.Error != "" {
			return Reply{}, errors.Newf("%s: %s", req.Model, chunk.Error)
		}
		reply.Content += chunk.Message.Content
		reply.Thinking += chunk.Message.Thinking
		for _, c := range chunk.Message.ToolCalls {
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				Name: c.Function.Name,
				Args: c.Function.Arguments,
			})
		}
		if on != nil && (chunk.Message.Content != "" || chunk.Message.Thinking != "") {
			on(Event{Content: chunk.Message.Content, Thinking: chunk.Message.Thinking})
		}
		if chunk.Done {
			reply.Done = chunk.DoneReason
			reply.EvalTokens = chunk.EvalCount
			reply.PromptTokens = chunk.PromptEvalCount
		}
	}
	if err := sc.Err(); err != nil {
		if starved {
			return Reply{}, errors.Newf("%s stopped producing for %s", req.Model, o.idle())
		}
		return Reply{}, errors.Wrapf(err, "reading the stream from %s", req.Model)
	}
	return reply, nil
}

// Allocated reports the context the host actually gave a model, from the list of
// what is loaded.
//
// It reports an error rather than a zero when it cannot tell. A silent zero was
// read as a real allocation once, because a model stored under a `:latest` tag
// never matched the untagged name it was asked about, and a whole run recorded
// a context of zero with nothing noticing.
func (o *Ollama) Allocated(ctx context.Context, model string) (int, error) {
	var out struct {
		Models []struct {
			Name          string `json:"name"`
			Model         string `json:"model"`
			ContextLength int    `json:"context_length"`
		} `json:"models"`
	}
	if err := o.get(ctx, "/api/ps", &out); err != nil {
		return 0, err
	}
	for _, m := range out.Models {
		// A host answers under either spelling depending on how the model was
		// pulled, and a caller has no way to know which.
		if sameModel(m.Name, model) || sameModel(m.Model, model) {
			return m.ContextLength, nil
		}
	}
	return 0, errors.Newf("%s is not loaded on %s", model, o.Addr)
}

// RequireContext refuses a run whose host gave the model less room than the work
// declares it needs.
//
// A loop that starts anyway is measuring a context it does not have, and the
// shortfall surfaces later as the model forgetting the file it just read.
func RequireContext(ctx context.Context, p Provider, model string, need int) error {
	got, err := p.Allocated(ctx, model)
	if err != nil {
		return errors.Wrapf(err, "checking the context given to %s", model)
	}
	if got < need {
		return errors.Newf("%s loaded with %d tokens of context and the work needs %d", model, got, need)
	}
	return nil
}

// sameModel compares model names, treating a missing tag as `:latest`, which is
// how a host stores one and not always how a caller names it.
func sameModel(a, b string) bool {
	return tagged(a) == tagged(b)
}

// tagged returns a model name with its tag, defaulting to latest.
func tagged(name string) string {
	if strings.Contains(name, ":") {
		return name
	}
	return name + ":latest"
}

// idle is the idle timeout, defaulted.
func (o *Ollama) idle() time.Duration {
	if o.IdleTimeout > 0 {
		return o.IdleTimeout
	}
	return DefaultIdleTimeout
}

// client is the HTTP client, defaulted.
func (o *Ollama) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return http.DefaultClient
}

// get reads a JSON document from the host.
func (o *Ollama) get(ctx context.Context, path string, out any) error {
	url := "http://" + o.Addr + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrapf(err, "building the request to %s", url)
	}
	res, err := o.client().Do(req)
	if err != nil {
		return errors.Wrapf(err, "calling %s", url)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.Newf("%s returned %s", url, res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return errors.Wrapf(err, "decoding %s", url)
	}
	return nil
}

// wire is the request as the host's chat route expects it.
func (o *Ollama) wire(req Request) map[string]any {
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		wm := map[string]any{"role": string(m.Role), "content": m.Content}
		if m.ToolName != "" {
			wm["tool_name"] = m.ToolName
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, c := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"function": map[string]any{"name": c.Name, "arguments": c.Args},
				})
			}
			wm["tool_calls"] = calls
		}
		msgs = append(msgs, wm)
	}

	opts := map[string]any{}
	if req.NumCtx > 0 {
		opts["num_ctx"] = req.NumCtx
	}
	if req.Predict > 0 {
		opts["num_predict"] = req.Predict
	}

	out := map[string]any{
		"model":      req.Model,
		"messages":   msgs,
		"stream":     true,
		"keep_alive": "30m",
	}
	if len(opts) > 0 {
		out["options"] = opts
	}
	if req.Think != "" {
		out["think"] = thinkField(req.Think)
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Schema,
				},
			})
		}
		out["tools"] = tools
	}
	return out
}

// thinkField renders the reasoning setting, which a host accepts as a boolean or
// as a level.
func thinkField(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	default:
		return s
	}
}

// chatChunk is one line of the streamed reply.
type chatChunk struct {
	Message struct {
		Content   string `json:"content"`
		Thinking  string `json:"thinking"`
		ToolCalls []struct {
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	EvalCount       int    `json:"eval_count"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	Error           string `json:"error"`
}

// compile-time proof that the host satisfies the interface the loop holds.
var _ Provider = (*Ollama)(nil)
