package kernel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The Jupyter wire protocol (v5.3). Each message on a shell/control/iopub/stdin
// socket is a multipart ZMQ message:
//
//	[ids...] <IDS|MSG> <signature> <header> <parent_header> <metadata> <content> [buffers...]
//
// "ids" are ZMQ routing identities (present on ROUTER sockets; a topic prefix on
// PUB). The signature is an HMAC of the four JSON frames after the delimiter.
const (
	delimiter       = "<IDS|MSG>"
	protocolVersion = "5.3"
)

// msgHeader is a Jupyter message header. The same struct is used for a message's
// own header and (parsed back) for the parent header.
type msgHeader struct {
	MsgID    string `json:"msg_id"`
	Session  string `json:"session"`
	Username string `json:"username"`
	Date     string `json:"date"`
	MsgType  string `json:"msg_type"`
	Version  string `json:"version"`
}

// message is a decoded wire message. The four signed frames are kept as raw
// bytes so an incoming header can be echoed verbatim as a reply's parent_header
// (faithful round-trip), while Header is the parsed form for dispatch.
type message struct {
	Identities [][]byte // ZMQ routing identities (ROUTER reply addressing)
	Header     msgHeader
	RawHeader  json.RawMessage
	Parent     json.RawMessage
	Metadata   json.RawMessage
	Content    json.RawMessage
}

// sign returns the hex HMAC-SHA256 of the concatenated frames under key. An
// empty key (unsecured connection) yields an empty signature, per the protocol.
func sign(key []byte, frames ...[]byte) string {
	if len(key) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	for _, f := range frames {
		mac.Write(f)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// decodeMessage parses and signature-verifies the frames of an inbound wire
// message. The leading frames before <IDS|MSG> are retained as routing
// identities for addressing the reply on a ROUTER socket.
func decodeMessage(frames [][]byte, key []byte) (*message, error) {
	d := -1
	for i, f := range frames {
		if string(f) == delimiter {
			d = i
			break
		}
	}
	if d < 0 {
		return nil, errors.New("no <IDS|MSG> delimiter in message")
	}
	if len(frames) < d+6 {
		return nil, fmt.Errorf("incomplete message: %d frames after delimiter, need 5", len(frames)-d-1)
	}
	sig := string(frames[d+1])
	hdr, parent, meta, content := frames[d+2], frames[d+3], frames[d+4], frames[d+5]
	if want := sign(key, hdr, parent, meta, content); !hmac.Equal([]byte(want), []byte(sig)) {
		return nil, errors.New("invalid HMAC signature")
	}
	m := &message{
		Identities: frames[:d],
		RawHeader:  append(json.RawMessage(nil), hdr...),
		Parent:     append(json.RawMessage(nil), parent...),
		Metadata:   append(json.RawMessage(nil), meta...),
		Content:    append(json.RawMessage(nil), content...),
	}
	if err := json.Unmarshal(hdr, &m.Header); err != nil {
		return nil, fmt.Errorf("bad header: %w", err)
	}
	return m, nil
}

// encodeWire builds the wire frames (delimiter onward) for an outbound message:
// a fresh header of msgType, the given parent header bytes, metadata, and
// content, signed under key. Routing identities / topic are prepended by the
// caller (the kernel) since they differ between ROUTER replies and PUB
// broadcasts.
func encodeWire(key []byte, msgType, session string, parent json.RawMessage, content, metadata any) ([][]byte, error) {
	hdr := msgHeader{
		MsgID:    newUUID(),
		Session:  session,
		Username: "kernel",
		Date:     time.Now().UTC().Format(time.RFC3339Nano),
		MsgType:  msgType,
		Version:  protocolVersion,
	}
	hb, err := json.Marshal(hdr)
	if err != nil {
		return nil, err
	}
	if len(parent) == 0 {
		parent = json.RawMessage("{}")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	mb, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	if content == nil {
		content = map[string]any{}
	}
	cb, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	sig := sign(key, hb, parent, mb, cb)
	return [][]byte{[]byte(delimiter), []byte(sig), hb, parent, mb, cb}, nil
}

// newUUID returns a random RFC-4122 version-4 UUID string, used for message ids.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail; fall back to a time-derived id rather than
		// panicking inside the kernel loop.
		return fmt.Sprintf("ass-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
