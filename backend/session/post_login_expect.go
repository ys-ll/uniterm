package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const postLoginExpectBufferLimit = 8192

type PostLoginExpectStep struct {
	Expect        string `json:"expect"`
	Send          string `json:"send"`
	Enter         bool   `json:"enter"`
	TimeoutSecond int    `json:"timeoutSecond,omitempty"`
}

type postLoginExpectAutomationConfig struct {
	Steps          []PostLoginExpectStep
	Variables      map[string]string
	Output         *postLoginOutputBuffer
	Send           func([]byte) error
	IsConnected    func() bool
	DefaultTimeout time.Duration
}

type postLoginOutputChunk struct {
	seq  uint64
	text string
}

type postLoginOutputBuffer struct {
	mu     sync.Mutex
	seq    uint64
	chunks []postLoginOutputChunk
	size   int
}

func newPostLoginOutputBuffer() *postLoginOutputBuffer {
	return &postLoginOutputBuffer{}
}

func (b *postLoginOutputBuffer) Append(data []byte) {
	if b == nil || len(data) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	text := string(data)
	b.chunks = append(b.chunks, postLoginOutputChunk{seq: b.seq, text: text})
	b.size += len(text)
	for b.size > postLoginExpectBufferLimit && len(b.chunks) > 0 {
		b.size -= len(b.chunks[0].text)
		b.chunks = b.chunks[1:]
	}
}

func (b *postLoginOutputBuffer) LatestSeq() uint64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

func (b *postLoginOutputBuffer) TextSince(afterSeq uint64) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	var out strings.Builder
	for _, chunk := range b.chunks {
		if chunk.seq > afterSeq {
			out.WriteString(chunk.text)
		}
	}
	return trimPostLoginOutput(out.String())
}

func runPostLoginExpectAutomation(ctx context.Context, config postLoginExpectAutomationConfig) error {
	if len(config.Steps) == 0 {
		return nil
	}
	if config.Send == nil {
		return fmt.Errorf("post-login expect send function is nil")
	}
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = 10 * time.Second
	}
	if config.IsConnected == nil {
		config.IsConnected = func() bool { return true }
	}
	if config.Output == nil {
		config.Output = newPostLoginOutputBuffer()
	}

	cursorSeq := uint64(0)
	for idx, step := range config.Steps {
		if !config.IsConnected() {
			return fmt.Errorf("post-login expect stopped: session disconnected")
		}

		timeout := config.DefaultTimeout
		if step.TimeoutSecond > 0 {
			timeout = time.Duration(step.TimeoutSecond) * time.Second
		}
		if err := waitForPostLoginExpect(ctx, config.Output, cursorSeq, step.Expect, timeout, config.IsConnected); err != nil {
			return fmt.Errorf("post-login expect step %d: %w", idx+1, err)
		}

		payload := expandPostLoginVariables(step.Send, config.Variables)
		if step.Enter {
			payload += "\r"
		}
		sendBoundarySeq := config.Output.LatestSeq()
		if payload != "" {
			if err := config.Send([]byte(payload)); err != nil {
				return fmt.Errorf("post-login expect step %d send: %w", idx+1, err)
			}
		}
		cursorSeq = sendBoundarySeq
	}
	return nil
}

func waitForPostLoginExpect(
	ctx context.Context,
	output *postLoginOutputBuffer,
	afterSeq uint64,
	expect string,
	timeout time.Duration,
	isConnected func() bool,
) error {
	if matchesPostLoginExpect(output.TextSince(afterSeq), expect) {
		return nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !isConnected() {
			return fmt.Errorf("session disconnected")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timeout waiting for %q", expect)
		case <-ticker.C:
			if matchesPostLoginExpect(output.TextSince(afterSeq), expect) {
				return nil
			}
		}
	}
}

func matchesPostLoginExpect(output, expect string) bool {
	if expect == "" {
		return true
	}
	return strings.Contains(strings.ToLower(output), strings.ToLower(expect))
}

func expandPostLoginVariables(input string, variables map[string]string) string {
	if input == "" || len(variables) == 0 {
		return input
	}
	out := input
	for key, value := range variables {
		out = strings.ReplaceAll(out, "${"+key+"}", value)
	}
	return out
}

func trimPostLoginOutput(output string) string {
	if len(output) <= postLoginExpectBufferLimit {
		return output
	}
	// Walk back from the trim boundary until DecodeLastRuneInString stops
	// reporting (RuneError, 1): that pair is returned exactly when its
	// input ends with an incomplete UTF-8 sequence. Bumping the cut point
	// past those trailing bytes preserves the rune rather than splitting it.
	start := len(output) - postLoginExpectBufferLimit
	for start > 0 {
		_, size := utf8.DecodeLastRuneInString(output[:start])
		if size > 1 {
			// Multi-byte rune consumed cleanly; we're sitting between runes.
			break
		}
		// size==1 either means ASCII or the tail of an invalid sequence.
		// An ASCII byte at start-1 means we're already on a rune boundary;
		// a continuation byte (0x80..0xBF) means we chopped mid-rune.
		if output[start-1]&0xC0 != 0x80 {
			break
		}
		start--
	}
	return output[start:]
}
