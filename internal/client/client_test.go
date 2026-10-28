package client

import (
	"strings"
	"testing"
)

func TestNormalizeServer(t *testing.T) {
	for in, want := range map[string]string{
		"trysort.com": "https://trysort.com", "https://TrySort.com/mcp?x=1": "https://trysort.com", "http://localhost:8001": "http://localhost:8001",
		"http://127.0.0.1:5179": "http://127.0.0.1:5179", "http://[::1]:8001": "http://[::1]:8001", "http://sort.localhost": "http://sort.localhost",
	} {
		if got, err := NormalizeServer(in); err != nil || got != want {
			t.Errorf("NormalizeServer(%q) = %q, %v", in, got, err)
		}
	}
	// Plain http:// would send the sign-in in clear text, so it is only for this computer.
	if _, err := NormalizeServer("http://203.0.113.7"); err == nil || !strings.Contains(err.Error(), "Use https://203.0.113.7") {
		t.Errorf("http to another computer: %v", err)
	}
	for _, bad := range []string{"http://", "ftp://trysort.com", "https://user:pass@trysort.com", "http://10.0.0.1:8001"} {
		if _, err := NormalizeServer(bad); err == nil {
			t.Errorf("NormalizeServer(%q) should fail", bad)
		}
	}
	if ServerFlag(DefaultServer) != "" || ServerFlag("http://x.test") != " --server http://x.test" {
		t.Error("ServerFlag")
	}
}

func TestTokenGoesOnlyToItsServer(t *testing.T) {
	t.Setenv("SORTDO_TOKEN", "secret")
	t.Setenv("SORTDO_SERVER", "")
	if token, err := AccessToken(DefaultServer); err != nil || token != "secret" {
		t.Fatalf("default server: %q %v", token, err)
	}
	if _, err := AccessToken("https://other.example"); err == nil || !strings.Contains(err.Error(), "does not send it to https://other.example") {
		t.Fatalf("other server: %v", err)
	}
	t.Setenv("SORTDO_SERVER", "http://localhost:8001")
	if token, err := AccessToken("http://localhost:8001"); err != nil || token != "secret" {
		t.Fatalf("SORTDO_SERVER: %q %v", token, err)
	}
	if _, err := AccessToken(DefaultServer); err == nil {
		t.Fatal("with SORTDO_SERVER set, the token went to the default server")
	}
}

func TestParseMCPResponse(t *testing.T) {
	for _, raw := range []string{`{"result":{"ok":true}}`, "event: message\ndata: {\"partial\":1}\n\ndata: {\"result\":{\"ok\":true}}\n\n"} {
		answer, err := ParseMCPResponse(raw)
		if err != nil || answer["result"].(map[string]any)["ok"] != true {
			t.Errorf("ParseMCPResponse(%q) = %v, %v", raw, answer, err)
		}
	}
	if _, err := ParseMCPResponse("<html>"); err == nil {
		t.Error("HTML is not an answer")
	}
	data := (&Result{Text: "plain"}).Data()
	if data["text"] != "plain" {
		t.Errorf("Data of plain text: %v", data)
	}
}
