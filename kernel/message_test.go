package kernel

import (
	"encoding/json"
	"testing"
)

func TestSignEmptyKey(t *testing.T) {
	if got := sign(nil, []byte("a"), []byte("b")); got != "" {
		t.Errorf("sign with empty key = %q, want empty", got)
	}
}

func TestSignDeterministic(t *testing.T) {
	key := []byte("secret")
	a := sign(key, []byte("x"), []byte("y"))
	b := sign(key, []byte("x"), []byte("y"))
	if a == "" || a != b {
		t.Errorf("sign not deterministic: %q vs %q", a, b)
	}
	// Per the protocol, the signature is HMAC over the concatenated frames, so a
	// different key must change it.
	if c := sign([]byte("other"), []byte("x"), []byte("y")); c == a {
		t.Errorf("sign should depend on the key")
	}
}

// TestEncodeDecodeRoundTrip builds a wire message, frames it the way the kernel
// frames a ROUTER reply (identity + wire), and decodes it back.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	key := []byte("hunter2")
	wire, err := encodeWire(key, "execute_reply", "sess-1", json.RawMessage(`{"msg_id":"parent-9"}`),
		map[string]any{"status": "ok", "execution_count": 3}, nil)
	if err != nil {
		t.Fatalf("encodeWire: %v", err)
	}
	// Prepend a routing identity, as a ROUTER socket delivers it on Recv.
	frames := append([][]byte{[]byte("ID")}, wire...)

	m, err := decodeMessage(frames, key)
	if err != nil {
		t.Fatalf("decodeMessage: %v", err)
	}
	if len(m.Identities) != 1 || string(m.Identities[0]) != "ID" {
		t.Errorf("identities = %v, want [ID]", m.Identities)
	}
	if m.Header.MsgType != "execute_reply" {
		t.Errorf("msg_type = %q, want execute_reply", m.Header.MsgType)
	}
	if m.Header.Session != "sess-1" {
		t.Errorf("session = %q, want sess-1", m.Header.Session)
	}
	if m.Header.Version != protocolVersion {
		t.Errorf("version = %q, want %q", m.Header.Version, protocolVersion)
	}
	var parent map[string]any
	if err := json.Unmarshal(m.Parent, &parent); err != nil {
		t.Fatalf("parent header: %v", err)
	}
	if parent["msg_id"] != "parent-9" {
		t.Errorf("parent msg_id = %v, want parent-9", parent["msg_id"])
	}
	var content map[string]any
	if err := json.Unmarshal(m.Content, &content); err != nil {
		t.Fatalf("content: %v", err)
	}
	if content["status"] != "ok" {
		t.Errorf("content status = %v, want ok", content["status"])
	}
}

func TestDecodeRejectsBadSignature(t *testing.T) {
	key := []byte("hunter2")
	wire, err := encodeWire(key, "kernel_info_request", "s", nil, nil, nil)
	if err != nil {
		t.Fatalf("encodeWire: %v", err)
	}
	frames := append([][]byte{[]byte("ID")}, wire...)
	// Decoding with a different key must fail the HMAC check.
	if _, err := decodeMessage(frames, []byte("wrong")); err == nil {
		t.Fatalf("expected signature rejection with wrong key")
	}
}

func TestDecodeMissingDelimiter(t *testing.T) {
	if _, err := decodeMessage([][]byte{[]byte("ID"), []byte("nope")}, nil); err == nil {
		t.Fatalf("expected error when <IDS|MSG> delimiter is absent")
	}
}

func TestNewUUIDFormat(t *testing.T) {
	u := newUUID()
	if len(u) != 36 || u[8] != '-' || u[13] != '-' || u[14] != '4' {
		t.Errorf("uuid %q is not a v4 UUID", u)
	}
}
