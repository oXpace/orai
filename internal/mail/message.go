package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	schema          = 1
	frontmatterHead = "---json\n"
	maxMessageSize  = 10 * 1024 * 1024
)

var Kinds = []string{"brainstorm", "review_request", "review_response", "question", "answer", "decision", "status", "todo"}

// Header mirrors AMQ's message frontmatter (field order and tags included).
type Header struct {
	Schema       int            `json:"schema"`
	ID           string         `json:"id"`
	From         string         `json:"from"`
	To           []string       `json:"to"`
	Thread       string         `json:"thread"`
	Subject      string         `json:"subject,omitempty"`
	Created      string         `json:"created"`
	Refs         []string       `json:"refs,omitempty"`
	Priority     string         `json:"priority,omitempty"`
	Kind         string         `json:"kind,omitempty"`
	Labels       []string       `json:"labels,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	ReplyTo      string         `json:"reply_to,omitempty"`
	ReplyProject string         `json:"reply_project,omitempty"`
	FromProject  string         `json:"from_project,omitempty"`
}

type Message struct {
	Header Header
	Body   string
}

func ValidKind(kind string) bool {
	if kind == "" {
		return true
	}
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func defaultPriority(kind string) string {
	switch kind {
	case "":
		return ""
	case "status":
		return "low"
	default:
		return "normal"
	}
}

// NewID follows AMQ: <UTC stamp>_pid<pid>_<8 hex>, sortable and collision-resistant.
func NewID(now time.Time) string {
	return fmt.Sprintf("%s_pid%d_%s", now.UTC().Format("2006-01-02T15-04-05.000Z"), os.Getpid(), randomHex(4))
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // the OS random source does not fail in practice
	}
	return hex.EncodeToString(buf)
}

func (m Message) Marshal() ([]byte, error) {
	if m.Header.Schema == 0 {
		m.Header.Schema = schema
	}
	header, err := json.MarshalIndent(m.Header, "", "  ")
	if err != nil {
		return nil, err
	}
	body := m.Body
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	var out bytes.Buffer
	out.WriteString(frontmatterHead)
	out.Write(header)
	out.WriteString("\n---\n")
	out.WriteString(body)
	return out.Bytes(), nil
}

var errFrontmatter = errors.New("not an AMQ message (frontmatter)")

func Parse(data []byte) (Message, error) {
	if len(data) > maxMessageSize {
		return Message{}, fmt.Errorf("message exceeds %d bytes", maxMessageSize)
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte(frontmatterHead)) {
		return Message{}, errFrontmatter
	}
	payload := data[len(frontmatterHead):]
	dec := json.NewDecoder(bytes.NewReader(payload))
	var header Header
	if err := dec.Decode(&header); err != nil {
		return Message{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	rest := bytes.TrimLeft(payload[dec.InputOffset():], " \t\r\n")
	if !bytes.HasPrefix(rest, []byte("---\n")) {
		return Message{}, errFrontmatter
	}
	return Message{Header: header, Body: string(rest[len("---\n"):])}, nil
}

func canonicalP2P(a, b string) string {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if b < a {
		a, b = b, a
	}
	return "p2p/" + a + "__" + b
}
