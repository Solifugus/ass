// Package kernel implements a Jupyter kernel for ASS. It speaks the Jupyter
// wire protocol (v5.3) over ZeroMQ — via the pure-Go github.com/go-zeromq/zmq4,
// so no C compiler is needed — and feeds each cell to a resident
// session.Session, so datasets, librefs, and macro state persist across cells
// exactly as in the REPL. The session is the seam: a cell is one Submit, its
// captured LOG+listing is streamed back as cell output.
package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"sync"

	"github.com/go-zeromq/zmq4"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/session"
)

// Kernel holds the live sockets, the resident session, and the execution state
// for one kernel process.
type Kernel struct {
	conn      connectionInfo
	key       []byte
	sessionID string

	sess *session.Session

	shell   zmq4.Socket
	control zmq4.Socket
	stdin   zmq4.Socket
	iopub   zmq4.Socket
	hb      zmq4.Socket

	iopubMu   sync.Mutex // serializes publishes (PUB Send is not concurrency-safe here)
	execCount int

	cancel context.CancelFunc
}

// Run starts a kernel from a Jupyter connection file and blocks until a
// shutdown_request is received (or a fatal socket error occurs).
func Run(connPath string) error {
	conn, err := loadConnection(connPath)
	if err != nil {
		return err
	}
	return serve(context.Background(), conn)
}

// serve binds the five sockets described by conn and runs the kernel until the
// context is cancelled (by ctx or a shutdown_request). Split from Run so tests
// can drive a kernel over real in-process sockets without a connection file.
func serve(parent context.Context, conn connectionInfo) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	k := &Kernel{
		conn:      conn,
		key:       []byte(conn.Key),
		sessionID: newUUID(),
		sess:      session.New(),
		cancel:    cancel,
	}

	k.shell = zmq4.NewRouter(ctx)
	k.control = zmq4.NewRouter(ctx)
	k.stdin = zmq4.NewRouter(ctx)
	k.iopub = zmq4.NewPub(ctx)
	k.hb = zmq4.NewRep(ctx)

	binds := []struct {
		sock zmq4.Socket
		port int
		name string
	}{
		{k.shell, conn.ShellPort, "shell"},
		{k.control, conn.ControlPort, "control"},
		{k.stdin, conn.StdinPort, "stdin"},
		{k.iopub, conn.IOPubPort, "iopub"},
		{k.hb, conn.HBPort, "heartbeat"},
	}
	for _, b := range binds {
		if err := b.sock.Listen(conn.endpoint(b.port)); err != nil {
			return fmt.Errorf("binding %s socket: %w", b.name, err)
		}
		defer b.sock.Close()
	}

	// Heartbeat and control run in the background; the shell loop runs in the
	// foreground and returns when the context is cancelled (shutdown).
	go k.heartbeatLoop(ctx)
	go k.routerLoop(ctx, k.control)

	k.publishStatus("starting", nil)
	k.routerLoop(ctx, k.shell)
	return nil
}

// heartbeatLoop echoes every message back on the REP socket, as the protocol's
// liveness check requires.
func (k *Kernel) heartbeatLoop(ctx context.Context) {
	for {
		msg, err := k.hb.Recv()
		if err != nil {
			return
		}
		if err := k.hb.Send(msg); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// routerLoop receives, verifies, and dispatches messages from a ROUTER socket
// (shell or control). Both carry the same request types; control exists so a
// frontend can shut down or interrupt even while shell is busy.
func (k *Kernel) routerLoop(ctx context.Context, sock zmq4.Socket) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		zm, err := sock.Recv()
		if err != nil {
			return
		}
		m, err := decodeMessage(zm.Frames, k.key)
		if err != nil {
			// A malformed or unsigned message is dropped, not fatal.
			continue
		}
		k.dispatch(sock, m)
	}
}

// dispatch wraps each request in the protocol's busy/idle status pair and routes
// it to the matching handler.
func (k *Kernel) dispatch(sock zmq4.Socket, m *message) {
	k.publishStatus("busy", m)
	defer k.publishStatus("idle", m)

	switch m.Header.MsgType {
	case "kernel_info_request":
		k.sendReply(sock, m, "kernel_info_reply", kernelInfo())
	case "execute_request":
		k.handleExecute(sock, m)
	case "is_complete_request":
		// We accept any fragment; the REPL-style "submit on run;/blank line" lives
		// in the frontend, so always report complete.
		k.sendReply(sock, m, "is_complete_reply", map[string]any{"status": "complete"})
	case "comm_info_request":
		k.sendReply(sock, m, "comm_info_reply", map[string]any{"comms": map[string]any{}})
	case "shutdown_request":
		var req struct {
			Restart bool `json:"restart"`
		}
		_ = json.Unmarshal(m.Content, &req)
		k.sendReply(sock, m, "shutdown_reply", map[string]any{"restart": req.Restart})
		k.cancel()
	case "interrupt_request":
		k.sendReply(sock, m, "interrupt_reply", map[string]any{})
	default:
		// comm_open/comm_msg/comm_close and anything else: ignore quietly.
	}
}

// handleExecute runs one cell against the resident session and streams the
// captured LOG+listing back as cell output.
func (k *Kernel) handleExecute(sock zmq4.Socket, m *message) {
	var req struct {
		Code   string `json:"code"`
		Silent bool   `json:"silent"`
	}
	_ = json.Unmarshal(m.Content, &req)

	if !req.Silent {
		k.execCount++
	}
	count := k.execCount

	k.publishIOPub("execute_input", m, map[string]any{
		"code":            req.Code,
		"execution_count": count,
	})

	// A rich sink keeps output in execution order: log lines and plain listings
	// accumulate into a text run streamed as stdout, while a tabular PROC result
	// (with an HTML rendering) flushes the pending text and is sent as
	// display_data so the notebook renders it as an HTML table. Outside a rich
	// frontend none of this path runs — see log.Logger.
	var pending strings.Builder
	flush := func() {
		if pending.Len() > 0 {
			if !req.Silent {
				text := pending.String()
				// Render the log/listing run as a styled, SAS-colored monospace
				// block (NOTE/WARNING/ERROR highlighted), with the raw text as the
				// text/plain fallback for non-HTML frontends.
				k.publishDisplayData(m, text, renderLogHTML(text))
			}
			pending.Reset()
		}
	}
	logger := log.NewSink(func(ev log.Event) {
		switch ev.Kind {
		case "table":
			if ev.HTML != "" {
				flush()
				if !req.Silent {
					k.publishDisplayData(m, ev.Text, ev.HTML)
				}
				return
			}
			pending.WriteString(ev.Text) // no HTML form: treat as plain text
		default: // "log", "listing"
			pending.WriteString(ev.Text)
		}
	})
	err := k.sess.Submit(req.Code, logger)
	flush()

	reply := k.executeReply(m, count, err, logger.ErrorCount())
	k.sendReply(sock, m, "execute_reply", reply)
}

// renderLogHTML wraps a run of log/listing text in a monospace block, coloring
// SAS log lines by severity (NOTE blue, WARNING green, ERROR red) the way the
// SAS log window does. Non-prefixed lines (PROC text, PUT output) are left in the
// theme's foreground color. Colors are chosen to read on both light and dark
// notebook themes.
func renderLogHTML(text string) string {
	var b strings.Builder
	b.WriteString(`<pre style="white-space:pre-wrap;word-break:break-word;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12.5px;line-height:1.4;margin:4px 0;color:inherit">`)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		esc := html.EscapeString(line)
		switch {
		case strings.HasPrefix(line, "ERROR"):
			b.WriteString(`<span style="color:#e0524a;font-weight:600">` + esc + `</span>`)
		case strings.HasPrefix(line, "WARNING"):
			b.WriteString(`<span style="color:#2f9e44;font-weight:600">` + esc + `</span>`)
		case strings.HasPrefix(line, "NOTE"):
			b.WriteString(`<span style="color:#4a8bf0">` + esc + `</span>`)
		default:
			b.WriteString(esc)
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteString("</pre>")
	return b.String()
}

// publishDisplayData emits a rich result on iopub with both an HTML rendering and
// a plain-text fallback, so notebooks show a table and plain frontends still get
// readable text.
func (k *Kernel) publishDisplayData(parent *message, text, html string) {
	k.publishIOPub("display_data", parent, map[string]any{
		"data": map[string]any{
			"text/plain": text,
			"text/html":  html,
		},
		"metadata":  map[string]any{},
		"transient": map[string]any{},
	})
}

// executeReply builds the execute_reply content and, on failure, also publishes
// the error to the iopub stderr stream so it is visible in the cell.
func (k *Kernel) executeReply(m *message, count int, err error, loggedErrors int) map[string]any {
	fail := func(ename, evalue string, traceback []string) map[string]any {
		k.publishStream(m, "stderr", strings.Join(traceback, "\n")+"\n")
		k.publishIOPub("error", m, map[string]any{
			"ename":     ename,
			"evalue":    evalue,
			"traceback": traceback,
		})
		return map[string]any{
			"status":          "error",
			"execution_count": count,
			"ename":           ename,
			"evalue":          evalue,
			"traceback":       traceback,
		}
	}

	var pe *session.ParseError
	switch {
	case err != nil && asParseError(err, &pe):
		return fail("ParseError", "parse error", pe.Errors)
	case err != nil:
		return fail("RuntimeError", err.Error(), []string{err.Error()})
	case loggedErrors > 0:
		// SAS-style: the step ran but logged ERRORs (e.g. a failing PROC PROOF).
		// The error text is already in the streamed output; mark the cell failed.
		msg := fmt.Sprintf("completed with %d error(s) — see the log above", loggedErrors)
		return fail("ProgramError", msg, []string{msg})
	default:
		return map[string]any{
			"status":           "ok",
			"execution_count":  count,
			"user_expressions": map[string]any{},
			"payload":          []any{},
		}
	}
}

// sendReply sends a reply on a ROUTER socket, re-using the request's routing
// identities to address the originating frontend.
func (k *Kernel) sendReply(sock zmq4.Socket, parent *message, msgType string, content any) {
	wire, err := encodeWire(k.key, msgType, k.replySession(parent), parent.RawHeader, content, nil)
	if err != nil {
		return
	}
	frames := append(append([][]byte{}, parent.Identities...), wire...)
	_ = sock.Send(zmq4.NewMsgFrom(frames...))
}

// publishIOPub broadcasts a message on the iopub PUB socket with a topic frame
// (the msg_type) so subscribing frontends can filter.
func (k *Kernel) publishIOPub(msgType string, parent *message, content any) {
	var parentHdr json.RawMessage
	if parent != nil {
		parentHdr = parent.RawHeader
	}
	wire, err := encodeWire(k.key, msgType, k.replySession(parent), parentHdr, content, nil)
	if err != nil {
		return
	}
	frames := append([][]byte{[]byte(msgType)}, wire...)
	k.iopubMu.Lock()
	_ = k.iopub.Send(zmq4.NewMsgFrom(frames...))
	k.iopubMu.Unlock()
}

func (k *Kernel) publishStatus(state string, parent *message) {
	k.publishIOPub("status", parent, map[string]any{"execution_state": state})
}

func (k *Kernel) publishStream(parent *message, name, text string) {
	k.publishIOPub("stream", parent, map[string]any{"name": name, "text": text})
}

// replySession returns the session id to stamp on outgoing headers: the client's
// session when known, else the kernel's own.
func (k *Kernel) replySession(parent *message) string {
	if parent != nil && parent.Header.Session != "" {
		return parent.Header.Session
	}
	return k.sessionID
}

// kernelInfo is the kernel_info_reply content describing the ASS/SAS language.
func kernelInfo() map[string]any {
	return map[string]any{
		"status":                 "ok",
		"protocol_version":       protocolVersion,
		"implementation":         "ass",
		"implementation_version": "0.1.0",
		"language_info": map[string]any{
			"name":           "sas",
			"version":        "",
			"mimetype":       "text/x-sas",
			"file_extension": ".sas",
			"pygments_lexer": "sas",
		},
		"banner":     "ASS — Analyst's Statistical Suite (SAS-compatible). Resident session; datasets and macro state persist across cells.",
		"help_links": []any{},
	}
}

// asParseError reports whether err is (or wraps) a *session.ParseError, storing
// it in target. Kept separate so message.go stays free of the session import.
func asParseError(err error, target **session.ParseError) bool {
	pe, ok := err.(*session.ParseError)
	if ok {
		*target = pe
	}
	return ok
}
