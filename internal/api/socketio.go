package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/model"
)

// SocketIOClient implements a minimal Socket.IO v0 client using xhr-polling.
type SocketIOClient struct {
	siteURL  string
	identity *auth.Identity
	client   *http.Client
	sid      string
	cookies  string // all cookies including GCLB for session stickiness
	ackID    int
}

// NewSocketIOClient creates a new Socket.IO client.
func NewSocketIOClient(siteURL string, identity *auth.Identity) *SocketIOClient {
	return &SocketIOClient{
		siteURL:  strings.TrimRight(siteURL, "/"),
		identity: identity,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cookies: identity.Cookies,
	}
}

// handshake performs the Socket.IO v0 handshake.
// Uses v2 scheme: passes projectId in query for automatic joinProjectResponse.
func (s *SocketIOClient) handshake(projectID string) error {
	u := fmt.Sprintf("%s/socket.io/1/?t=%d&projectId=%s",
		s.siteURL, time.Now().UnixMilli(), projectID)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", s.identity.Cookies)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	defer resp.Body.Close()

	// Capture GCLB cookie for session stickiness (Google Cloud LB)
	for _, c := range resp.Header.Values("Set-Cookie") {
		if name, val, ok := parseCookieHeader(c); ok {
			s.cookies = s.identity.Cookies + "; " + name + "=" + val
			_ = val
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Response format: sid:heartbeatTimeout:closeTimeout:transports
	parts := strings.SplitN(string(body), ":", 4)
	if len(parts) < 1 || parts[0] == "" {
		return fmt.Errorf("handshake: unexpected response: %s", string(body))
	}

	s.sid = parts[0]
	return nil
}

// poll sends a GET to receive messages from the server.
func (s *SocketIOClient) poll(projectID string) (string, error) {
	u := fmt.Sprintf("%s/socket.io/1/xhr-polling/%s?t=%d",
		s.siteURL, url.PathEscape(s.sid), time.Now().UnixMilli())

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", s.cookies)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// send sends a message to the server via POST.
func (s *SocketIOClient) send(msg string) error {
	u := fmt.Sprintf("%s/socket.io/1/xhr-polling/%s?t=%d",
		s.siteURL, url.PathEscape(s.sid), time.Now().UnixMilli())

	req, err := http.NewRequest("POST", u, strings.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", s.cookies)
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)

	return nil
}

// JoinProject connects to a project and returns the full project metadata.
// Uses the v2 scheme where projectId is passed in the handshake query,
// and the server automatically sends a joinProjectResponse event.
func (s *SocketIOClient) JoinProject(projectID string) (*model.ProjectMeta, error) {
	if err := s.handshake(projectID); err != nil {
		return nil, err
	}

	// Poll for connect message (type 1) and joinProjectResponse (type 5 event)
	for range 20 {
		data, err := s.poll(projectID)
		if err != nil {
			return nil, fmt.Errorf("poll: %w", err)
		}

		// Parse Socket.IO v0 messages
		// Format: type:id:endpoint:data
		// Multi-message: \ufffd<len>\ufffd<msg>\ufffd<len>\ufffd<msg>
		messages := parseMessages(data)
		for _, msg := range messages {
			if msg.Type == 5 { // Event
				var evt struct {
					Name string            `json:"name"`
					Args []json.RawMessage `json:"args"`
				}
				if err := json.Unmarshal([]byte(msg.Data), &evt); err != nil {
					continue
				}

				if evt.Name == "joinProjectResponse" && len(evt.Args) > 0 {
					var joinResp struct {
						Project model.ProjectMeta `json:"project"`
					}
					if err := json.Unmarshal(evt.Args[0], &joinResp); err != nil {
						return nil, fmt.Errorf("parse joinProjectResponse: %w", err)
					}
					return &joinResp.Project, nil
				}

				if evt.Name == "connectionRejected" {
					return nil, fmt.Errorf("connection rejected by server")
				}
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for joinProjectResponse")
}

// JoinDoc retrieves the content of a document.
func (s *SocketIOClient) JoinDoc(projectID, docID string) (string, int, error) {
	s.ackID++
	ackID := s.ackID

	// Socket.IO v0 event with ack: 5:ackId+::{"name":"joinDoc","args":["docId",{"encodeRanges":true}]}
	msg := fmt.Sprintf(`5:%d+::{"name":"joinDoc","args":["%s",{"encodeRanges":true}]}`, ackID, docID)
	if err := s.send(msg); err != nil {
		return "", 0, fmt.Errorf("send joinDoc: %w", err)
	}

	ackPrefix := fmt.Sprintf("6:::%d+", ackID)

	for range 20 {
		data, err := s.poll(projectID)
		if err != nil {
			return "", 0, fmt.Errorf("poll joinDoc: %w", err)
		}

		messages := parseMessages(data)
		for _, msg := range messages {
			raw := msg.Raw
			if msg.Type == 6 && strings.Contains(raw, ackPrefix) {
				// Ack response format: 6:::ackId+[lines, version, updates, ranges]
				idx := strings.Index(raw, "[")
				if idx == -1 {
					continue
				}
				jsonData := raw[idx:]
				end := findMatchingBracket(jsonData)
				if end == -1 {
					continue
				}
				jsonData = jsonData[:end+1]

				var response []json.RawMessage
				if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
					continue
				}

				// Response format: [error, lines, version, updates, ranges]
				// error is null on success
				if len(response) < 3 {
					continue
				}

				// Check for error (first element)
				if string(response[0]) != "null" {
					return "", 0, fmt.Errorf("joinDoc error: %s", string(response[0]))
				}

				var lines []string
				if err := json.Unmarshal(response[1], &lines); err != nil {
					return "", 0, fmt.Errorf("parse doc lines: %w", err)
				}

				var version int
				json.Unmarshal(response[2], &version)

				return strings.Join(lines, "\n"), version, nil
			}
		}
	}

	return "", 0, fmt.Errorf("timeout waiting for joinDoc response")
}

// LeaveDoc tells the server we're done with a document.
func (s *SocketIOClient) LeaveDoc(projectID, docID string) error {
	s.ackID++
	msg := fmt.Sprintf(`5:%d+::{"name":"leaveDoc","args":["%s"]}`, s.ackID, docID)
	return s.send(msg)
}

// Disconnect sends a disconnect message.
func (s *SocketIOClient) Disconnect() {
	s.send("0::")
}

// SendHeartbeat sends a heartbeat to keep the connection alive.
func (s *SocketIOClient) SendHeartbeat() error {
	return s.send("2::")
}

// ==========================================================================
// Socket.IO v0 message parsing
// ==========================================================================

type sioMessage struct {
	Type     int    // 0=disconnect, 1=connect, 2=heartbeat, 3=message, 4=json, 5=event, 6=ack, 7=error
	ID       string
	Endpoint string
	Data     string
	Raw      string
}

func parseMessages(data string) []sioMessage {
	if data == "" {
		return nil
	}

	// Multi-message format uses \ufffd as delimiter
	if strings.ContainsRune(data, '\ufffd') {
		var messages []sioMessage
		parts := strings.Split(data, "\ufffd")
		for i := 0; i < len(parts); i++ {
			// Skip length fields
			if len(parts[i]) == 0 {
				continue
			}
			// If this looks like a length (all digits), skip it
			isLen := true
			for _, c := range parts[i] {
				if c < '0' || c > '9' {
					isLen = false
					break
				}
			}
			if isLen {
				continue
			}
			if msg, ok := parseSingleMessage(parts[i]); ok {
				messages = append(messages, msg)
			}
		}
		return messages
	}

	if msg, ok := parseSingleMessage(data); ok {
		return []sioMessage{msg}
	}
	return nil
}

func parseSingleMessage(raw string) (sioMessage, bool) {
	msg := sioMessage{Raw: raw}

	if len(raw) == 0 {
		return msg, false
	}

	msg.Type = int(raw[0] - '0')
	if msg.Type < 0 || msg.Type > 8 {
		return msg, false
	}

	// Format: type:id:endpoint:data
	// Split the full string into 4 parts
	parts := strings.SplitN(raw, ":", 4)
	if len(parts) >= 2 {
		msg.ID = parts[1]
	}
	if len(parts) >= 3 {
		msg.Endpoint = parts[2]
	}
	if len(parts) >= 4 {
		msg.Data = parts[3]
	}

	return msg, true
}

func parseCookieHeader(header string) (string, string, bool) {
	parts := strings.SplitN(header, ";", 2)
	if len(parts) == 0 {
		return "", "", false
	}
	kv := strings.SplitN(parts[0], "=", 2)
	if len(kv) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]), true
}

func findMatchingBracket(s string) int {
	if len(s) == 0 || s[0] != '[' {
		return -1
	}
	depth := 0
	inString := false
	escaped := false
	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
