package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aead.dev/minisign"
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

// testKey stands in for the release key: the tests sign with it, and sortdo trusts it while they run.
func testKey(t *testing.T) minisign.PrivateKey {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := public.MarshalText()
	saved := releaseKeys
	releaseKeys = []string{string(text)}
	t.Cleanup(func() { releaseKeys = saved })
	return private
}

// sign signs as tools/sign does.
func sign(key minisign.PrivateKey, sums []byte, version string) []byte {
	reader := minisign.NewReader(bytes.NewReader(sums))
	_, _ = io.Copy(io.Discard, reader)
	return reader.SignWithComments(key, SignedComment(version), "test")
}

func checksums(sum string) []byte {
	return []byte(fmt.Sprintf("%s  sortdo_linux_amd64.tar.gz\nabc  sortdo_darwin_arm64.tar.gz\n", sum))
}

// releaseServer serves release 1.4.0 with its checksums and, when signature is not nil, its signature.
func releaseServer(t *testing.T, body []byte, sum string, signature []byte) {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v1.4.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/download/v1.4.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(checksums(sum))
	})
	if signature != nil {
		mux.HandleFunc("/releases/download/v1.4.0/checksums.txt.minisig", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(signature) })
	}
	mux.HandleFunc("/releases/download/v1.4.0/sortdo_linux_amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("SORTDO_RELEASES_URL", server.URL)
}

func TestFetchAndReplace(t *testing.T) {
	key := testKey(t)
	body := archive(t, "new program")
	sum := sha256.Sum256(body)
	releaseServer(t, body, hex.EncodeToString(sum[:]), sign(key, checksums(hex.EncodeToString(sum[:])), "1.4.0"))
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
	if info, _ := os.Stat(executable); runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode())
	}
}

func TestChecksumMismatch(t *testing.T) {
	key := testKey(t)
	releaseServer(t, archive(t, "tampered"), strings.Repeat("0", 64), sign(key, checksums(strings.Repeat("0", 64)), "1.4.0"))
	if _, err := Fetch("1.4.0", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "does not match its checksum") {
		t.Fatalf("a wrong checksum stops the update: %v", err)
	}
	if _, err := Fetch("1.4.0", "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "no entry for sortdo_windows_amd64.zip") {
		t.Fatalf("a missing checksum stops the update: %v", err)
	}
}

// sortdo update trusts checksums.txt only with a signature from the release key, made for that version.
func TestSignature(t *testing.T) {
	key := testKey(t)
	body := archive(t, "new program")
	digest := sha256.Sum256(body)
	sum := hex.EncodeToString(digest[:])
	_, otherKey, _ := minisign.GenerateKey(rand.Reader)
	for _, c := range []struct {
		name      string
		signature []byte
		want      string
	}{
		{"unsigned", nil, "the release has no signature"},
		{"another key", sign(otherKey, checksums(sum), "1.4.0"), "release key did not sign"},
		{"an old release's signature", sign(key, checksums(sum), "1.3.0"), `is for "sortdo 1.3.0 checksums.txt"`},
		{"changed checksums", sign(key, checksums(strings.Repeat("1", 64)), "1.4.0"), "release key did not sign"},
		{"not a signature", []byte("hello"), "not a minisign signature"},
	} {
		releaseServer(t, body, sum, c.signature)
		if _, err := Fetch("1.4.0", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "nothing was changed") {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// The keys built into sortdo must be real minisign public keys.
func TestReleaseKeys(t *testing.T) {
	if len(releaseKeys) == 0 {
		t.Fatal("no release key")
	}
	for _, text := range releaseKeys {
		var key minisign.PublicKey
		if err := key.UnmarshalText([]byte(text)); err != nil {
			t.Errorf("release key %q: %v", text, err)
		}
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

func TestCopies(t *testing.T) {
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	name := "sortdo"
	if runtime.GOOS == "windows" {
		name = "sortdo.exe"
	}
	root := t.TempDir()
	local := filepath.Join(root, "local", name)
	write(local)
	other := filepath.Join(root, "other", name)
	write(other)
	path := strings.Join([]string{filepath.Dir(local), filepath.Join(root, "empty"), filepath.Dir(other), filepath.Dir(local)}, string(os.PathListSeparator))
	copies := Copies(path, other)
	if len(copies) != 2 || copies[0].Path != local || copies[0].Self || copies[1].Path != other || !copies[1].Self {
		t.Fatalf("copies: %+v", copies)
	}
	if copies := Copies(filepath.Dir(local), local); len(copies) != 1 || !copies[0].Self {
		t.Fatalf("one copy: %+v", copies)
	}
	if runtime.GOOS == "windows" {
		return // npm makes .cmd launchers on Windows, not links
	}
	// npm links its launcher into bin; the launcher starts the program inside the same package, so they are one copy.
	launcher := filepath.Join(root, "lib", "node_modules", "sortdo", "npm", "sortdo.js")
	program := filepath.Join(root, "lib", "node_modules", "sortdo", "node_modules", "@sortdo", "cli-linux-x64", "bin", "sortdo")
	write(launcher)
	write(program)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(launcher, filepath.Join(bin, "sortdo")); err != nil {
		t.Fatal(err)
	}
	path = strings.Join([]string{filepath.Dir(local), bin}, string(os.PathListSeparator))
	if copies := Copies(path, program); len(copies) != 2 || copies[0].Self || copies[1].Path != filepath.Join(bin, "sortdo") || !copies[1].Self {
		t.Fatalf("npm copies: %+v", copies)
	}
}
