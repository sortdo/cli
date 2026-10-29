package store

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestFileStorage(t *testing.T) {
	t.Setenv("SORTDO_CONFIG_DIR", t.TempDir()+"/sortdo")
	t.Setenv("SORTDO_KEYRING", "0")
	origin := "https://trysort.com"
	if tokens, _ := LoadTokens(origin); tokens != nil {
		t.Fatal("no sign-in yet")
	}
	place, keychainError, err := SaveTokens(origin, Tokens{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now()})
	if err != nil || keychainError != nil || place != CredentialsPath() {
		t.Fatalf("SaveTokens: %s %v %v", place, keychainError, err)
	}
	if info, _ := os.Stat(Dir()); runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", info.Mode())
	}
	if tokens, source := LoadTokens(origin); tokens == nil || tokens.AccessToken != "a" || source != CredentialsPath() {
		t.Fatalf("LoadTokens: %v %s", tokens, source)
	}
	if tokens, _ := LoadTokens("https://other.test"); tokens != nil {
		t.Fatal("tokens are per server")
	}
	DeleteTokens(origin)
	if tokens, _ := LoadTokens(origin); tokens != nil {
		t.Fatal("deleted")
	}
	_ = SavePending(origin, &Pending{UserCode: "BCDF-GHJK"})
	if pending, ok := LoadPending(origin); !ok || pending.UserCode != "BCDF-GHJK" {
		t.Fatal("pending")
	}
	_ = SavePending(origin, nil)
	if _, ok := LoadPending(origin); ok {
		t.Fatal("pending removed")
	}
}
