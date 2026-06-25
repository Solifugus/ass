package kernel

import (
	"encoding/json"
	"fmt"
	"os"
)

// connectionInfo is the Jupyter connection file written by the frontend and
// passed to the kernel on launch. It names the five socket ports and the HMAC
// signing key.
type connectionInfo struct {
	Transport       string `json:"transport"`
	IP              string `json:"ip"`
	ShellPort       int    `json:"shell_port"`
	IOPubPort       int    `json:"iopub_port"`
	StdinPort       int    `json:"stdin_port"`
	ControlPort     int    `json:"control_port"`
	HBPort          int    `json:"hb_port"`
	SignatureScheme string `json:"signature_scheme"`
	Key             string `json:"key"`
}

// loadConnection reads and parses a Jupyter connection file.
func loadConnection(path string) (connectionInfo, error) {
	var c connectionInfo
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("parsing connection file %s: %w", path, err)
	}
	if c.Transport == "" {
		c.Transport = "tcp"
	}
	if c.SignatureScheme != "" && c.SignatureScheme != "hmac-sha256" {
		return c, fmt.Errorf("unsupported signature scheme %q (only hmac-sha256)", c.SignatureScheme)
	}
	return c, nil
}

// endpoint builds a ZMQ endpoint string for a port, e.g. "tcp://127.0.0.1:54321".
func (c connectionInfo) endpoint(port int) string {
	return fmt.Sprintf("%s://%s:%d", c.Transport, c.IP, port)
}
