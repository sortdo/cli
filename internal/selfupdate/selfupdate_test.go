package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func archive(t *testing.T, program string) []byte {
	var buffer bytes.Buffer
	zipped := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(zipped)
	for _, file := range []struct{ name, body string }{{"README.md", "readme"}, {"sortdo", program}} {
		_ = writer.WriteHeader(&tar.Header{Name: file.name, Mode: 0o755, Size: int64(len(file.body)), Typeflag: tar.TypeReg})
		_, _ = writer.Write([]byte(file.body))
	}
	writer.Close()
	zipped.Close()
	return buffer.Bytes()
}

func releaseServer(t *testing.T, body []byte, sum string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v1.4.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/v1.4.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  sortdo_linux_amd64.tar.gz\nabc  sortdo_darwin_arm64.tar.gz\n", sum)
	})
	mux.HandleFunc("/releases/download/v1.4.0/sortdo_linux_amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("SORTDO_RELEASES_URL", server.URL)
}

func TestFetchAndReplace(t *testing.T) {
	body := archive(t, "new program")
	sum := sha256.Sum256(body)
	releaseServer(t, body, hex.EncodeToString(sum[:]))
	if latest, err := Latest(5 * time.Second); err != nil || latest != "1.4.0" {
		t.Fatalf("Latest = %q, %v", latest, err)
	}
	program, err := Fetch("1.4.0", "linux", "amd64")
	if err != nil || string(program) != "new program" {
		t.Fatalf("Fetch = %q, %v", program, err)
	}
	executable := filepath.Join(t.TempDir(), "sortdo")
	_ = os.WriteFile(executable, []byte("old program"), 0o755)
	if err := Replace(executable, program); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(executable); string(data) != "new program" {
		t.Fatalf("replaced: %q", data)
	}
	if info, _ := os.Stat(executable); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
}

func TestChecksumMismatch(t *testing.T) {
	releaseServer(t, archive(t, "tampered"), strings.Repeat("0", 64))
	if _, err := Fetch("1.4.0", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "does not match its checksum") {
		t.Fatalf("a wrong checksum stops the update: %v", err)
	}
	if _, err := Fetch("1.4.0", "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "no entry for sortdo_windows_amd64.zip") {
		t.Fatalf("a missing checksum stops the update: %v", err)
	}
}

func TestVersionsAndMethods(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{{"1.10.0", "1.9.9", true}, {"1.2.3", "1.2.3", false}, {"v2.0.0", "1.99.0", true}, {"1.2.3", "1.2.4", false}} {
		if Newer(c.a, c.b) != c.newer {
			t.Errorf("Newer(%s, %s) != %v", c.a, c.b, c.newer)
		}
	}
	for path, want := range map[string]Method{
		"/usr/local/lib/node_modules/sortdo/node_modules/@sortdo/cli-linux-x64/bin/sortdo": Npm,
		"/opt/homebrew/Caskroom/sortdo/1.2.3/sortdo":                                       Homebrew,
		"/usr/local/Caskroom/sortdo/1.2.3/sortdo":                                          Homebrew,
		"/home/linuxbrew/.linuxbrew/Cellar/sortdo/1.2.3/bin/sortdo":                        Homebrew,
		`C:\Users\ada\AppData\Local\Microsoft\WinGet\Packages\Sort.sortdo\sortdo.exe`:      Winget,
		"/home/ada/.local/bin/sortdo":                                                      Script,
	} {
		if got := DetectMethod(path); got != want {
			t.Errorf("DetectMethod(%s) = %s, want %s", path, got, want)
		}
	}
}
