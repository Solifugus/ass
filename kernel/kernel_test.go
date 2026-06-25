package kernel

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/session"
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

	// Drain iopub, accumulating all rich output. The colored log (with the
	// dataset NOTE) and the PROC PRINT table arrive as separate display_data
	// messages, each with an HTML rendering and a text/plain fallback.
	htmlAll, plainAll := "", ""
	sawTable := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !sawTable {
		m, ok := recvWithin(t, iopub, key, 500*time.Millisecond)
		if !ok {
			continue
		}
		switch m.Header.MsgType {
		case "stream":
			var c struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(m.Content, &c)
			plainAll += c.Text
		case "display_data":
			var c struct {
				Data map[string]string `json:"data"`
			}
			_ = json.Unmarshal(m.Content, &c)
			htmlAll += c.Data["text/html"]
			plainAll += c.Data["text/plain"]
			if contains(c.Data["text/html"], "<table") {
				sawTable = true
			}
		}
	}
	// The colored log carries the dataset-creation NOTE (as a colored span and in
	// the text/plain fallback).
	if !contains(plainAll, "WORK.T has 3 observations") {
		t.Errorf("rich output missing the NOTE; got:\n%s", plainAll)
	}
	if !contains(htmlAll, "<pre") || !contains(htmlAll, "color:") {
		t.Errorf("expected a styled (colored) log <pre>; got:\n%s", htmlAll)
	}
	// The PROC PRINT table is a styled HTML table with a plain-text fallback.
	if !contains(htmlAll, "<table") || !contains(htmlAll, "<td") {
		t.Errorf("display_data text/html is not an HTML table; got:\n%s", htmlAll)
	}
	if !contains(htmlAll, "WORK.T") {
		t.Errorf("table caption (WORK.T) missing; got:\n%s", htmlAll)
	}
	if !contains(plainAll, "Obs") {
		t.Errorf("text/plain fallback missing the listing; got:\n%s", plainAll)
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

// TestWriteSampleHTML assembles a realistic notebook-cell rendering (colored log
// + styled tables) into an HTML file, so the visual design can be eyeballed in a
// browser. Gated behind ASS_WRITE_SAMPLE so it is inert in normal test runs:
//
//	ASS_WRITE_SAMPLE=/tmp/ass-sample.html go test ./kernel/ -run SampleHTML
func TestWriteSampleHTML(t *testing.T) {
	out := os.Getenv("ASS_WRITE_SAMPLE")
	if out == "" {
		t.Skip("set ASS_WRITE_SAMPLE=<path> to write the sample page")
	}
	program := `
data sales;
  input region $ product $ units price;
  revenue = units * price;
  datalines;
East Widget 120 4.50
East Gadget 80 12.00
West Widget 200 4.50
West Gadget 150 12.00
South Widget 95 4.50
;
run;
title "Quarterly Sales Report";
title2 "by Region and Product";
proc print data=sales; run;
proc means data=sales; var units revenue; run;
proc freq data=sales; tables region; run;
proc freq data=sales; tables region*product / chisq; run;
proc proof data=sales;
  notnull region product;
  values region in ("East" "West" "South");
  range units 0 - 150;
  rule "revenue positive": revenue > 0;
run;
`
	// Drive a session with the same rich sink the kernel uses.
	var page strings.Builder
	page.WriteString(`<html><body style="background:#fff;padding:24px;max-width:900px;margin:auto">`)
	page.WriteString(`<h2 style="font-family:sans-serif">ASS — sample notebook cell output</h2>`)
	var pending strings.Builder
	flush := func() {
		if pending.Len() > 0 {
			page.WriteString(renderLogHTML(pending.String()))
			pending.Reset()
		}
	}
	logger := log.NewSink(func(ev log.Event) {
		if ev.Kind == "table" && ev.HTML != "" {
			flush()
			page.WriteString(ev.HTML)
			return
		}
		pending.WriteString(ev.Text)
	})
	if err := session.New().Submit(program, logger); err != nil {
		t.Fatalf("submit: %v", err)
	}
	flush()
	// Also show the same page on a dark background to check theme-robustness.
	page.WriteString(`</body></html>`)
	if err := os.WriteFile(out, []byte(page.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote sample HTML to %s", out)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
