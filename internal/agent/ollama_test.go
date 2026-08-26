package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// chatting returns a host whose chat route writes the given lines, flushing each
// one, with a pause before every line after the first.
func chatting(t *testing.T, pause time.Duration, lines []string) *Ollama {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		for i, line := range lines {
			if i > 0 {
				time.Sleep(pause)
			}
			fmt.Fprintln(w, line)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	o := NewOllama(strings.TrimPrefix(srv.URL, "http://"))
	return o
}

// TestAStreamIsAssembledFromItsChunks covers the shape of a reply that arrives in
// pieces: text, reasoning and a tool call each accumulate, and the last chunk
// carries the counts.
func TestAStreamIsAssembledFromItsChunks(t *testing.T) {
	g := NewWithT(t)
	o := chatting(t, 0, []string{
		`{"message":{"content":"one ","thinking":"first "}}`,
		`{"message":{"content":"two","thinking":"second"}}`,
		`{"message":{"tool_calls":[{"function":{"name":"read","arguments":{"path":"a.go"}}}]}}`,
		`{"message":{},"done":true,"done_reason":"stop","eval_count":12,"prompt_eval_count":34}`,
	})
	var seen []Event
	reply, err := o.Stream(t.Context(), Request{Model: "m"}, func(e Event) {
		seen = append(seen, e)
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reply.Content).To(Equal("one two"))
	g.Expect(reply.Thinking).To(Equal("first second"))
	g.Expect(reply.Done).To(Equal("stop"))
	g.Expect(reply.EvalTokens).To(Equal(12))
	g.Expect(reply.PromptTokens).To(Equal(34))
	g.Expect(reply.ToolCalls).To(HaveLen(1))
	g.Expect(reply.ToolCalls[0].Name).To(Equal("read"))
	g.Expect(reply.ToolCalls[0].Args).To(HaveKeyWithValue("path", "a.go"))
	g.Expect(seen).To(HaveLen(2), "a chunk carrying only a tool call is not progress text")
}

// TestTheIdleTimerResetsPerToken is the reason the timeout is not on the request.
//
// A model that reasons for minutes and a dead socket look the same to a deadline
// on the whole call. A stream that is still producing is alive, however slowly,
// so the timer restarts on every chunk and only silence ends the turn.
func TestTheIdleTimerResetsPerToken(t *testing.T) {
	g := NewWithT(t)
	o := chatting(t, 40*time.Millisecond, []string{
		`{"message":{"content":"a"}}`,
		`{"message":{"content":"b"}}`,
		`{"message":{"content":"c"}}`,
		`{"message":{"content":"d"}}`,
		`{"message":{},"done":true,"done_reason":"stop"}`,
	})
	o.IdleTimeout = 100 * time.Millisecond

	start := time.Now()
	reply, err := o.Stream(t.Context(), Request{Model: "m"}, nil)
	elapsed := time.Since(start)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reply.Content).To(Equal("abcd"))
	g.Expect(elapsed).
		To(BeNumerically(">", o.IdleTimeout), "the whole stream outlasted the idle timeout and was not cut off")
}

// TestASilentStreamIsAbandoned is the other half: silence past the timeout ends
// the turn, and the error blames the model rather than the transport.
//
// The two messages are kept apart on purpose. A stream that never produced
// anything and one that stopped halfway are different failures: the first is
// usually a model that will not start, the second a model that stalled with the
// answer half written, and the reply text of the second is worth looking at.
func TestASilentStreamIsAbandoned(t *testing.T) {
	t.Run("stalls after producing", func(t *testing.T) {
		g := NewWithT(t)
		o := chatting(t, 300*time.Millisecond, []string{
			`{"message":{"content":"a"}}`,
			`{"message":{},"done":true,"done_reason":"stop"}`,
		})
		o.IdleTimeout = 50 * time.Millisecond

		_, err := o.Stream(t.Context(), Request{Model: "m"}, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("stopped producing"))
	})

	t.Run("never produces at all", func(t *testing.T) {
		g := NewWithT(t)
		held := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-held
		}))
		defer srv.Close()
		defer close(held)
		o := NewOllama(strings.TrimPrefix(srv.URL, "http://"))
		o.IdleTimeout = 50 * time.Millisecond

		_, err := o.Stream(t.Context(), Request{Model: "m"}, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("produced nothing"))
	})
}

// TestAllocatedFindsAModelHoweverItIsTagged is the recorded failure this guards.
//
// A host stores an untagged pull under `:latest`, so an exact-name match never
// fires, and the caller reads a zero as a real allocation. A whole run recorded a
// context of zero with nothing noticing.
func TestAllocatedFindsAModelHoweverItIsTagged(t *testing.T) {
	cases := []struct {
		name   string
		loaded string
		ask    string
		want   int
		wantOK bool
	}{
		{
			name:   "untagged ask, tagged on the host",
			loaded: `{"name":"glm:latest","model":"glm:latest","context_length":20480}`,
			ask:    "glm",
			want:   20480,
			wantOK: true,
		},
		{
			name:   "tagged ask, untagged on the host",
			loaded: `{"name":"glm","model":"glm","context_length":8192}`,
			ask:    "glm:latest",
			want:   8192,
			wantOK: true,
		},
		{
			name:   "a different model entirely",
			loaded: `{"name":"other:latest","model":"other:latest","context_length":4096}`,
			ask:    "glm",
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"models":[%s]}`, c.loaded)
			}))
			defer srv.Close()
			o := NewOllama(strings.TrimPrefix(srv.URL, "http://"))

			got, err := o.Allocated(t.Context(), c.ask)
			if !c.wantOK {
				g.Expect(err).To(HaveOccurred(), "an unloaded model must not report a context")
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(c.want))
		})
	}
}

// TestRequireContextRefusesAShortfall stops a run that would measure a context it
// was not given. The shortfall otherwise surfaces later as the model forgetting
// the file it just read.
func TestRequireContextRefusesAShortfall(t *testing.T) {
	cases := []struct {
		name    string
		have    int
		need    int
		wantErr string
	}{
		{name: "enough", have: 20480, need: 20480},
		{name: "more than enough", have: 65536, need: 20480},
		{name: "short", have: 4096, need: 20480, wantErr: "needs 20480"},
		{name: "not loaded", have: 0, need: 20480, wantErr: "not loaded"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			err := RequireContext(t.Context(), &Script{Context: c.have}, "m", c.need)
			if c.wantErr == "" {
				g.Expect(err).NotTo(HaveOccurred())
				return
			}
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(c.wantErr))
		})
	}
}

// TestTheRequestCarriesWhatWasAskedFor pins the wire shape, which is the one part
// a host silently ignores rather than rejecting.
func TestTheRequestCarriesWhatWasAskedFor(t *testing.T) {
	g := NewWithT(t)
	o := &Ollama{}
	wire := o.wire(Request{
		Model:    "m",
		NumCtx:   20480,
		Predict:  512,
		Think:    "false",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "read a file",
			Schema:      map[string]any{"type": "object"},
		}},
	})

	g.Expect(wire).To(HaveKeyWithValue("stream", true))
	g.Expect(wire).To(HaveKeyWithValue("think", false), "off is a boolean, and is not the same as absent")
	opts, _ := wire["options"].(map[string]any)
	g.Expect(opts).To(HaveKeyWithValue("num_ctx", 20480))
	g.Expect(opts).To(HaveKeyWithValue("num_predict", 512))

	bare := o.wire(Request{Model: "m"})
	g.Expect(bare).NotTo(HaveKey("think"), "an empty setting leaves the field off entirely")
	g.Expect(bare).NotTo(HaveKey("options"), "an unset context must not be sent as a zero")
}

// TestAnAddressGetsAPortWithoutBreakingIPv6 is why the port is added by
// SplitHostPort rather than by looking for a colon. A bare IPv6 literal is full
// of colons and carries no port, so the naive check leaves it alone and the URL
// built from it does not parse.
func TestAnAddressGetsAPortWithoutBreakingIPv6(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "a host with no port", in: "gpu", want: "gpu:11434"},
		{name: "a host with a port", in: "gpu:1234", want: "gpu:1234"},
		{name: "an IPv4 with no port", in: "192.0.2.1", want: "192.0.2.1:11434"},
		{name: "an IPv4 with a port", in: "192.0.2.1:1234", want: "192.0.2.1:1234"},
		{name: "a bare IPv6", in: "::1", want: "[::1]:11434"},
		{name: "a bracketed IPv6 with a port", in: "[::1]:1234", want: "[::1]:1234"},
		{name: "a full IPv6", in: "2001:db8::1", want: "[2001:db8::1]:11434"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(withPort(c.in, ollamaPort)).To(Equal(c.want))
		})
	}
}
