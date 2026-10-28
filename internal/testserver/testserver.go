// Package testserver is a small fake Sort server for tests: the device grant, refresh, revoke and MCP.
package testserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Server records what sortdo sent and answers like Sort.
type Server struct {
	*httptest.Server
	mu sync.Mutex
	// Approved makes the next device poll succeed; Denied makes it fail.
	Approved, Denied bool
	Polls            int
	DeviceRequests   []map[string]string
	Calls            []map[string]any // tools/call params
	Revoked          []string
	Refreshes        int
	// Stream answers MCP requests as an event stream instead of JSON.
	Stream bool
	tokens int
}

// Tools is the tool list the server offers.
var Tools = []map[string]any{
	{"name": "search", "title": "Search", "description": "Search pages.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []any{"query"}}},
	{"name": "manage_teams", "title": "Teams", "description": "View and manage teams.", "inputSchema": map[string]any{"type": "object", "required": []any{"action"}, "properties": map[string]any{
		"action":      map[string]any{"type": "string", "enum": []any{"list", "add_members", "update"}},
		"teamId":      map[string]any{"type": "string", "description": "Team ID. For add_members."},
		"memberIds":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Members. For add_members."},
		"permissions": map[string]any{"type": "object"},
		"limit":       map[string]any{"type": "integer"},
		"visibility":  map[string]any{"type": "string", "enum": []any{"public", "private"}},
		"description": map[string]any{"type": "string"},
	}}},
	{"name": "manage_preferences", "title": "Preferences", "description": "Preferences.", "inputSchema": map[string]any{"type": "object", "required": []any{"action"}, "properties": map[string]any{
		"action":   map[string]any{"type": "string", "enum": []any{"get", "update"}},
		"aiMemory": map[string]any{"type": "boolean"},
	}}},
	{"name": "do_search", "title": "Search more", "description": "Search with filters.", "inputSchema": map[string]any{"type": "object", "required": []any{"query", "type"}, "properties": map[string]any{
		"query": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"},
	}}},
}

// New starts the server; it stops when the test ends.
func New(t *testing.T) *Server {
	server := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/device_authorization", server.device)
	mux.HandleFunc("/oauth/token", server.token)
	mux.HandleFunc("/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		server.mu.Lock()
		server.Revoked = append(server.Revoked, r.Form.Get("token"))
		server.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/mcp", server.mcp)
	server.Server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	request := map[string]string{}
	for key := range r.Form {
		request[key] = r.Form.Get(key)
	}
	s.mu.Lock()
	s.DeviceRequests = append(s.DeviceRequests, request)
	s.mu.Unlock()
	if request["client_id"] != "sortdo" {
		writeJSON(w, 400, map[string]any{"error": "invalid_client"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"device_code": "device-1", "user_code": "BCDF-GHJK", "verification_uri": s.URL + "/app/connect",
		"verification_uri_complete": s.URL + "/app/connect?code=BCDF-GHJK", "expires_in": 900, "interval": 1,
	})
}

func (s *Server) newTokens() map[string]any {
	s.tokens++
	return map[string]any{"access_token": fmt.Sprintf("access-%d", s.tokens), "refresh_token": fmt.Sprintf("refresh-%d", s.tokens), "expires_in": 3600, "token_type": "Bearer"}
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Form.Get("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		s.Polls++
		switch {
		case s.Polls == 1:
			writeJSON(w, 400, map[string]any{"error": "slow_down"})
		case s.Denied:
			writeJSON(w, 400, map[string]any{"error": "access_denied"})
		case !s.Approved:
			writeJSON(w, 400, map[string]any{"error": "authorization_pending"})
		default:
			writeJSON(w, 200, s.newTokens())
		}
	case "refresh_token":
		s.Refreshes++
		if strings.HasPrefix(r.Form.Get("refresh_token"), "refresh-") {
			writeJSON(w, 200, s.newTokens())
		} else {
			writeJSON(w, 400, map[string]any{"error": "invalid_grant"})
		}
	default:
		writeJSON(w, 400, map[string]any{"error": "unsupported_grant_type"})
	}
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("authorization"), "Bearer access-") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var request struct {
		ID     any            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	_ = json.Unmarshal(body, &request)
	var result any
	switch request.Method {
	case "tools/list":
		result = map[string]any{"tools": Tools}
	case "tools/call":
		s.mu.Lock()
		s.Calls = append(s.Calls, request.Params)
		s.mu.Unlock()
		name, _ := request.Params["name"].(string)
		arguments, _ := json.Marshal(request.Params["arguments"])
		switch name {
		case "manage_workspace":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"workspace":{"name":"Alpha"}}`}}}
		case "search":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "found"}}, "structuredContent": map[string]any{"results": []any{map[string]any{"title": "Notes", "url": "https://trysort.com/alpha/1"}}}}
		case "create_page":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": `Created page "Notes" with id: 01a18253-6129-77df-964c-7a4c6a4f2235 in Everyone. It is a draft.`}}}
		case "no_such_tool":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "Unknown tool"}}, "isError": true}
		default:
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(arguments)}}}
		}
	}
	answer, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	if s.Stream {
		w.Header().Set("content-type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", answer)
		return
	}
	w.Header().Set("content-type", "application/json")
	_, _ = w.Write(answer)
}

// LastCall is the arguments of the last tools/call.
func (s *Server) LastCall() (string, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Calls) == 0 {
		return "", nil
	}
	last := s.Calls[len(s.Calls)-1]
	name, _ := last["name"].(string)
	arguments, _ := last["arguments"].(map[string]any)
	return name, arguments
}
