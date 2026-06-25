package kernel

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/go-zeromq/zmq4"
)

// freePort grabs an available localhost TCP port. Racy in principle, fine for a
// loopback test.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// recvWithin reads one decoded message from sock, failing if none arrives in d.
func recvWithin(t *testing.T, sock zmq4.Socket, key []byte, d time.Duration) (*message, bool) {
	t.Helper()
	type res struct {
		m   *message
		err error
	}
	ch := make(chan res, 1)
	go func() {
		zm, err := sock.Recv()
		if err != nil {
			ch <- res{nil, err}
			return
		}
		m, err := decodeMessage(zm.Frames, key)
		ch <- res{m, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, false
		}
		return r.m, true
	case <-time.After(d):
		return nil, false
	}
}

func TestKernelEndToEnd(t *testing.T) {
	key := []byte("test-key")
	conn := connectionInfo{
		Transport:       "tcp",
		IP:              "127.0.0.1",
		ShellPort:       freePort(t),
		IOPubPort:       freePort(t),
		StdinPort:       freePort(t),
		ControlPort:     freePort(t),
		HBPort:          freePort(t),
		SignatureScheme: "hmac-sha256",
		Key:             string(key),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = serve(ctx, conn) }()

	// Client sockets: DEALER on shell, SUB on iopub.
	shell := zmq4.NewDealer(ctx)
	if err := shell.Dial(conn.endpoint(conn.ShellPort)); err != nil {
		t.Fatalf("dial shell: %v", err)
	}
	defer shell.Close()
	iopub := zmq4.NewSub(ctx)
	if err := iopub.Dial(conn.endpoint(conn.IOPubPort)); err != nil {
		t.Fatalf("dial iopub: %v", err)
	}
	if err := iopub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer iopub.Close()

	send := func(msgType string, content any) {
		wire, err := encodeWire(key, msgType, "client-session", nil, content, nil)
		if err != nil {
			t.Fatalf("encodeWire: %v", err)
		}
		if err := shell.Send(zmq4.NewMsgFrom(wire...)); err != nil {
			t.Fatalf("send %s: %v", msgType, err)
		}
	}

	// Establish the iopub pipe despite PUB/SUB slow-joiner: resend kernel_info
	// until a broadcast arrives on iopub.
	gotIO := false
	for i := 0; i < 25 && !gotIO; i++ {
		send("kernel_info_request", map[string]any{})
		if _, ok := recvWithin(t, iopub, key, 200*time.Millisecond); ok {
			gotIO = true
		}
	}
	if !gotIO {
		t.Fatal("never received any iopub broadcast; pipe not established")
	}

	// kernel_info_reply must come back on shell.
	if !waitForReply(t, shell, key, "kernel_info_reply", 2*time.Second) {
		t.Fatal("no kernel_info_reply")
	}

	// Execute a two-step program: build a dataset, then print it.
	code := "data t;\n input x; datalines;\n1\n2\n3\n;\nrun;\nproc print data=t; run;"
	send("execute_request", map[string]any{
		"code": code, "silent": false, "store_history": true,
		"allow_stdin": false, "stop_on_error": true,
	})

	// Drain iopub until we see the stdout stream carrying the PROC PRINT output.
	streamText := ""
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && streamText == "" {
		m, ok := recvWithin(t, iopub, key, 500*time.Millisecond)
		if !ok {
			continue
		}
		if m.Header.MsgType == "stream" {
			var c struct {
				Name string `json:"name"`
				Text string `json:"text"`
			}
			_ = json.Unmarshal(m.Content, &c)
			if c.Name == "stdout" {
				streamText = c.Text
			}
		}
	}
	if streamText == "" {
		t.Fatal("no stdout stream from execute_request")
	}
	for _, want := range []string{"WORK.T has 3 observations", "Obs"} {
		if !contains(streamText, want) {
			t.Errorf("stream output missing %q; got:\n%s", want, streamText)
		}
	}

	// execute_reply with status ok and execution_count 1.
	reply := readReply(t, shell, key, "execute_reply", 3*time.Second)
	if reply == nil {
		t.Fatal("no execute_reply")
	}
	var rc struct {
		Status         string `json:"status"`
		ExecutionCount int    `json:"execution_count"`
	}
	_ = json.Unmarshal(reply.Content, &rc)
	if rc.Status != "ok" {
		t.Errorf("execute_reply status = %q, want ok", rc.Status)
	}
	if rc.ExecutionCount != 1 {
		t.Errorf("execution_count = %d, want 1", rc.ExecutionCount)
	}

	// Shutdown cleanly.
	send("shutdown_request", map[string]any{"restart": false})
	_ = waitForReply(t, shell, key, "shutdown_reply", 2*time.Second)
}

// waitForReply reads shell replies until one of msgType arrives or time runs out.
func waitForReply(t *testing.T, sock zmq4.Socket, key []byte, msgType string, d time.Duration) bool {
	return readReply(t, sock, key, msgType, d) != nil
}

func readReply(t *testing.T, sock zmq4.Socket, key []byte, msgType string, d time.Duration) *message {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		m, ok := recvWithin(t, sock, key, 500*time.Millisecond)
		if !ok {
			continue
		}
		if m.Header.MsgType == msgType {
			return m
		}
	}
	return nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
