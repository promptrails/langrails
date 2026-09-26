package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// stopGrace is how long Close waits for the server to exit after its stdin
// is closed before killing it.
const stopGrace = 2 * time.Second

// errServerExited is returned for calls still waiting when the server
// process exits or closes its stdout.
var errServerExited = errors.New("mcp server process exited")

// stdioTransport speaks newline-delimited JSON-RPC over a child process's
// stdin and stdout, as the MCP stdio transport specifies.
type stdioTransport struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex
	nextID  atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan jsonRPCResponse
	done    chan struct{} // closed when the reader stops
	err     error         // why the reader stopped

	closeOnce sync.Once
}

func startStdio(command string, args, env []string, dir string, stderr io.Writer) (*stdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stderr = stderr // nil discards

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	t := &stdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		pending: map[int64]chan jsonRPCResponse{},
		done:    make(chan struct{}),
	}
	go t.read(stdout)
	return t, nil
}

// incoming is any message the server sends: a response (id + result or
// error), a request (id + method) or a notification (method only).
type incoming struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *jsonRPCError   `json:"error,omitempty"`
}

func (t *stdioTransport) read(stdout io.Reader) {
	r := bufio.NewReader(stdout)
	var readErr error
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			t.dispatch(line)
		}
		if err != nil {
			readErr = err
			break
		}
	}

	t.mu.Lock()
	t.err = errServerExited
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		t.err = fmt.Errorf("%w: %v", errServerExited, readErr)
	}
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.mu.Unlock()
	close(t.done)
}

func (t *stdioTransport) dispatch(line []byte) {
	var msg incoming
	if err := json.Unmarshal(line, &msg); err != nil {
		return // not JSON-RPC (a stray log line); ignore
	}

	switch {
	case msg.Method != "" && len(msg.ID) > 0:
		// A request from the server. Answer ping; refuse the rest, since
		// this client offers no capabilities (sampling, roots, ...).
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": msg.ID}
		if msg.Method == "ping" {
			resp["result"] = map[string]interface{}{}
		} else {
			resp["error"] = jsonRPCError{Code: -32601, Message: "method not found"}
		}
		_ = t.write(resp)
	case msg.Method != "":
		// Notification (progress, logging, list_changed); nothing to do.
	default:
		var id int64
		if err := json.Unmarshal(msg.ID, &id); err != nil {
			return
		}
		t.mu.Lock()
		ch, ok := t.pending[id]
		delete(t.pending, id)
		t.mu.Unlock()
		if ok {
			ch <- jsonRPCResponse{ID: id, Result: msg.Result, Error: msg.Error}
		}
	}
}

func (t *stdioTransport) write(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err = t.stdin.Write(append(b, '\n'))
	return err
}

func (t *stdioTransport) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	ch := make(chan jsonRPCResponse, 1)

	t.mu.Lock()
	if t.err != nil {
		err := t.err
		t.mu.Unlock()
		return nil, err
	}
	t.pending[id] = ch
	t.mu.Unlock()

	if err := t.write(jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		t.forget(id)
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			t.mu.Lock()
			err := t.err
			t.mu.Unlock()
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		t.forget(id)
		_ = t.write(jsonRPCNotification{JSONRPC: "2.0", Method: "notifications/cancelled",
			Params: map[string]interface{}{"requestId": id}})
		return nil, ctx.Err()
	}
}

func (t *stdioTransport) forget(id int64) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

func (t *stdioTransport) notify(_ context.Context, method string, params interface{}) error {
	return t.write(jsonRPCNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// close shuts the server down the way the spec asks: close its stdin, give
// it a moment to exit, then kill it.
func (t *stdioTransport) close() error {
	t.closeOnce.Do(func() {
		_ = t.stdin.Close()
		exited := make(chan error, 1)
		go func() { exited <- t.cmd.Wait() }()
		select {
		case <-exited:
		case <-time.After(stopGrace):
			_ = t.cmd.Process.Kill()
			<-exited
		}
		<-t.done
	})
	return nil
}
