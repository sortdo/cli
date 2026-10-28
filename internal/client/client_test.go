package client

import "testing"

func TestNormalizeServer(t *testing.T) {
	for in, want := range map[string]string{
		"trysort.com": "https://trysort.com", "https://TrySort.com/mcp?x=1": "https://trysort.com", "http://localhost:8001": "http://localhost:8001",
	} {
		if got, err := NormalizeServer(in); err != nil || got != want {
			t.Errorf("NormalizeServer(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"http://", "ftp://trysort.com", "https://user:pass@trysort.com"} {
		if _, err := NormalizeServer(bad); err == nil {
			t.Errorf("NormalizeServer(%q) should fail", bad)
		}
	}
	if ServerFlag(DefaultServer) != "" || ServerFlag("http://x.test") != " --server http://x.test" {
		t.Error("ServerFlag")
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
