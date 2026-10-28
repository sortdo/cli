// Package client talks to a Sort server: the OAuth device grant, token refresh and revocation, and MCP tool calls.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sortdo/cli/internal/store"
)

const (
	// ClientID is sortdo's fixed first-party client. The server knows it without registration and shows the person
	// that Sort made it.
	ClientID        = "sortdo"
	DefaultServer   = "https://trysort.com"
	deviceGrant     = "urn:ietf:params:oauth:grant-type:device_code"
	protocolVersion = "2025-11-25"
)

// Error is a message for the person, printed as it is.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// Errorf makes an Error.
func Errorf(format string, args ...any) error { return &Error{fmt.Sprintf(format, args...)} }

// Version is sortdo's version, sent in the User-Agent header.
var Version = "dev"

var httpClient = &http.Client{Timeout: 60 * time.Second}

func userAgent() string {
	return "sortdo/" + Version + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}

// NormalizeServer turns "trysort.com" or "https://trysort.com/x" into an origin such as "https://trysort.com".
func NormalizeServer(server string) (string, error) {
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	parsed, err := url.Parse(server)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
		return "", Errorf("Not a server address: %s", server)
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

// ServerFlag is " --server <origin>" for any server but the default, so printed commands work as they are.
func ServerFlag(origin string) string {
	if origin == DefaultServer {
		return ""
	}
	return " --server " + origin
}

func send(request *http.Request) (int, map[string]any, error) {
	request.Header.Set("accept", "application/json")
	request.Header.Set("user-agent", userAgent())
	response, err := httpClient.Do(request)
	if err != nil {
		return 0, nil, Errorf("Could not reach %s: %v", request.URL.Host, unwrap(err))
	}
	defer response.Body.Close()
	body := map[string]any{}
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = json.Unmarshal(data, &body)
	return response.StatusCode, body, nil
}

func unwrap(err error) error {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return urlError.Err
	}
	return err
}

func postForm(endpoint string, values url.Values) (int, map[string]any, error) {
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	return send(request)
}

// Text returns a string field of a JSON object, or "".
func Text(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return value
}

func number(body map[string]any, key string, fallback int) int {
	if value, ok := body[key].(float64); ok && value > 0 {
		return int(value)
	}
	return fallback
}

func reason(body map[string]any, status int) string {
	if text := Text(body, "error_description"); text != "" {
		return text
	}
	if text := Text(body, "error"); text != "" {
		return text
	}
	return fmt.Sprintf("HTTP %d", status)
}

// Hints are what the person said about themselves, so the sign-in page can fill them in.
type Hints struct {
	Email         string
	WorkspaceName string
}

// StartDevice asks the server for a device code and the link the person opens.
func StartDevice(origin string, hints Hints) (*store.Pending, error) {
	values := url.Values{"client_id": {ClientID}, "scope": {"sort"}}
	if hints.Email != "" {
		values.Set("login_hint", hints.Email)
	}
	if hints.WorkspaceName != "" {
		values.Set("workspace_name", hints.WorkspaceName)
	}
	status, body, err := postForm(origin+"/oauth/device_authorization", values)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, Errorf("Could not start the sign-in: %s", reason(body, status))
	}
	return &store.Pending{
		ClientID: ClientID, DeviceCode: Text(body, "device_code"), UserCode: Text(body, "user_code"), Link: Text(body, "verification_uri_complete"),
		Interval: number(body, "interval", 5), ExpiresAt: time.Now().Add(time.Duration(number(body, "expires_in", 900)) * time.Second).UTC(),
	}, nil
}

// Sleep waits between polls. Tests replace it.
var Sleep = time.Sleep

// PollDevice waits for the person's decision and saves the tokens. It returns where they went.
func PollDevice(origin string, pending *store.Pending) (place string, keychainError error, err error) {
	interval := max(pending.Interval, 1)
	for time.Now().Before(pending.ExpiresAt) {
		Sleep(time.Duration(interval) * time.Second)
		status, body, err := postForm(origin+"/oauth/token", url.Values{"grant_type": {deviceGrant}, "device_code": {pending.DeviceCode}, "client_id": {pending.ClientID}})
		if err != nil {
			return "", nil, err
		}
		if status == http.StatusOK && Text(body, "access_token") != "" {
			return store.SaveTokens(origin, tokensFrom(body))
		}
		switch Text(body, "error") {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "access_denied":
			return "", nil, Errorf("The sign-in was cancelled in the browser.")
		case "expired_token":
			return "", nil, Errorf("The code expired. Run sortdo login%s again.", ServerFlag(origin))
		}
		return "", nil, Errorf("The sign-in failed: %s", reason(body, status))
	}
	return "", nil, Errorf("The code expired. Run sortdo login%s again.", ServerFlag(origin))
}

func tokensFrom(body map[string]any) store.Tokens {
	return store.Tokens{
		AccessToken: Text(body, "access_token"), RefreshToken: Text(body, "refresh_token"),
		ExpiresAt: time.Now().Add(time.Duration(number(body, "expires_in", 3600)) * time.Second).UTC(),
	}
}

// TokenSource says where AccessToken got its token: "SORTDO_TOKEN", "OS keychain" or a file path.
func TokenSource(origin string) string {
	if os.Getenv("SORTDO_TOKEN") != "" {
		return "SORTDO_TOKEN"
	}
	_, source := store.LoadTokens(origin)
	return source
}

// AccessToken returns SORTDO_TOKEN, or the stored token, refreshed when it expires within a minute.
func AccessToken(origin string) (string, error) {
	if token := os.Getenv("SORTDO_TOKEN"); token != "" {
		return token, nil
	}
	current, _ := store.LoadTokens(origin)
	if current == nil {
		return "", Errorf("Not signed in to %s. Run: sortdo login%s", origin, ServerFlag(origin))
	}
	if time.Until(current.ExpiresAt) > time.Minute {
		return current.AccessToken, nil
	}
	status, body, err := postForm(origin+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {current.RefreshToken}, "client_id": {ClientID}})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || Text(body, "access_token") == "" {
		store.DeleteTokens(origin)
		return "", Errorf("The sign-in has ended. Run sortdo login%s again.", ServerFlag(origin))
	}
	refreshed := tokensFrom(body)
	if _, _, err := store.SaveTokens(origin, refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// Revoke ends the stored sign-in on the server and removes it here. It reports whether there was one.
func Revoke(origin string) bool {
	current, _ := store.LoadTokens(origin)
	if current == nil {
		return false
	}
	_, _, _ = postForm(origin+"/oauth/revoke", url.Values{"token": {current.RefreshToken}, "client_id": {ClientID}})
	store.DeleteTokens(origin)
	return true
}

// ParseMCPResponse reads a JSON-RPC answer sent as JSON or as an event stream (the last data line).
func ParseMCPResponse(raw string) (map[string]any, error) {
	text := raw
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "data:") {
			text = strings.TrimSpace(line[5:])
		}
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, Errorf("The server sent an answer sortdo could not read: %.200s", raw)
	}
	return payload, nil
}

// RPC sends one MCP JSON-RPC request as the signed-in member.
func RPC(origin, method string, params map[string]any) (map[string]any, error) {
	token, err := AccessToken(origin)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": time.Now().UnixMilli(), "method": method, "params": params})
	request, err := http.NewRequest(http.MethodPost, origin+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("authorization", "Bearer "+token)
	request.Header.Set("content-type", "application/json")
	request.Header.Set("accept", "application/json, text/event-stream")
	request.Header.Set("mcp-protocol-version", protocolVersion)
	request.Header.Set("user-agent", userAgent())
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, Errorf("Could not reach %s: %v", request.URL.Host, unwrap(err))
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized:
		if os.Getenv("SORTDO_TOKEN") != "" {
			return nil, Errorf("The server refused SORTDO_TOKEN. Use a current access token, or unset it and run sortdo login.")
		}
		return nil, Errorf("The sign-in is no longer valid. Run sortdo login%s again.", ServerFlag(origin))
	case http.StatusTooManyRequests:
		return nil, Errorf("Too many requests. Wait a moment and try again.")
	}
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	answer, err := ParseMCPResponse(string(raw))
	if err != nil {
		return nil, err
	}
	if problem, ok := answer["error"].(map[string]any); ok {
		message := Text(problem, "message")
		if message == "" {
			message = "The server refused the request."
		}
		return nil, Errorf("%s", message)
	}
	result, _ := answer["result"].(map[string]any)
	return result, nil
}

// Tool is one tool from tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// ListTools returns the tools the member can use.
func ListTools(origin string) ([]Tool, error) {
	result, err := RPC(origin, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(result["tools"])
	var tools []Tool
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, Errorf("The server sent a tool list sortdo could not read.")
	}
	return tools, nil
}

// Result is a tool's answer: its text, its structured content when it has one, and whether it failed.
type Result struct {
	Text       string
	Structured any
	IsError    bool
}

// CallTool calls one tool with its arguments.
func CallTool(origin, name string, arguments map[string]any) (*Result, error) {
	result, err := RPC(origin, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return nil, err
	}
	var parts []string
	content, _ := result["content"].([]any)
	for _, item := range content {
		if part, ok := item.(map[string]any); ok && part["type"] == "text" {
			parts = append(parts, Text(part, "text"))
		}
	}
	isError, _ := result["isError"].(bool)
	return &Result{Text: strings.Join(parts, "\n"), Structured: result["structuredContent"], IsError: isError}, nil
}

// Data is the result as a JSON object: its structured content, its text read as JSON, or {"text": text}.
func (result *Result) Data() map[string]any {
	if data, ok := result.Structured.(map[string]any); ok {
		return data
	}
	data := map[string]any{}
	if json.Unmarshal([]byte(result.Text), &data) != nil {
		return map[string]any{"text": result.Text}
	}
	return data
}
