// Package selfupdate finds the latest sortdo release on GitHub and replaces the running program with it.
//
// Releases live at https://github.com/sortdo/cli/releases with fixed asset names (sortdo_<os>_<arch>.tar.gz, .zip on
// Windows) and a checksums.txt with their SHA-256 sums. The latest version comes from the redirect of
// /releases/latest, which needs no GitHub API call and has no rate limit.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repository is where releases are published. SORTDO_RELEASES_URL replaces it (tests, mirrors).
func Repository() string {
	if base := os.Getenv("SORTDO_RELEASES_URL"); base != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://github.com/sortdo/cli"
}

var noRedirects = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

var downloads = &http.Client{Timeout: 5 * time.Minute}

// Latest returns the latest released version, without a leading "v".
func Latest(timeout time.Duration) (string, error) {
	request, err := http.NewRequest(http.MethodHead, Repository()+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	client := *noRedirects
	client.Timeout = timeout
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	location := response.Header.Get("Location")
	tag := path.Base(location)
	if response.StatusCode < 300 || response.StatusCode >= 400 || !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("no release found at %s", Repository())
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// Newer reports whether version a is newer than version b. Both are X.Y.Z; a pre-release part is ignored.
func Newer(a, b string) bool {
	parse := func(version string) [3]int {
		var parts [3]int
		core, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "-")
		for index, field := range strings.SplitN(core, ".", 3) {
			parts[index], _ = strconv.Atoi(field)
		}
		return parts
	}
	left, right := parse(a), parse(b)
	for index := range left {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return false
}

// Method is how sortdo was installed. Only the install script's copy updates itself.
type Method string

const (
	Script   Method = "script"
	Npm      Method = "npm"
	Homebrew Method = "homebrew"
	Winget   Method = "winget"
)

// DetectMethod tells from the program's path which tool installed it.
func DetectMethod(executable string) Method {
	normalized := strings.ToLower(strings.ReplaceAll(executable, `\`, "/"))
	switch {
	case strings.Contains(normalized, "/node_modules/"):
		return Npm
	case strings.Contains(normalized, "/cellar/") || strings.Contains(normalized, "/caskroom/") || strings.Contains(normalized, "/homebrew/") || strings.Contains(normalized, "/linuxbrew/"):
		return Homebrew
	case strings.Contains(normalized, "/winget/"):
		return Winget
	}
	return Script
}

// Command is the update command for the install method.
func (m Method) Command() string {
	switch m {
	case Npm:
		return "npm install -g sortdo@latest"
	case Homebrew:
		return "brew upgrade sortdo"
	case Winget:
		return "winget upgrade Sort.sortdo"
	}
	return "sortdo update"
}

// UninstallCommand removes a copy that npm, Homebrew or winget installed.
func (m Method) UninstallCommand() string {
	switch m {
	case Npm:
		return "npm uninstall -g sortdo"
	case Homebrew:
		return "brew uninstall sortdo"
	case Winget:
		return "winget uninstall Sort.sortdo"
	}
	return ""
}

// Copy is one sortdo on PATH.
type Copy struct {
	Path string // as PATH finds it
	Self bool   // the program that runs now
}

// Copies lists the sortdo programs on PATH, first to last: a shell runs the first one. Links count once, and npm's
// launcher counts as the program it starts.
func Copies(pathList, self string) []Copy {
	name := "sortdo"
	if runtime.GOOS == "windows" {
		name = "sortdo.exe"
	}
	selfKey := copyKey(self)
	var copies []Copy
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		resolved := candidate
		if target, err := filepath.EvalSymlinks(candidate); err == nil {
			resolved = target
		}
		key := copyKey(resolved)
		if seen[key] {
			continue
		}
		seen[key] = true
		copies = append(copies, Copy{Path: candidate, Self: key == selfKey})
	}
	return copies
}

func copyKey(path string) string {
	normalized := strings.ReplaceAll(filepath.Clean(path), `\`, "/")
	if runtime.GOOS != "linux" {
		normalized = strings.ToLower(normalized)
	}
	if at := strings.Index(normalized, "/node_modules/sortdo/"); at >= 0 {
		return normalized[:at+len("/node_modules/sortdo")]
	}
	return normalized
}

// AssetName is the release archive for this system.
func AssetName(goos, goarch string) string {
	if goos == "windows" {
		return fmt.Sprintf("sortdo_%s_%s.zip", goos, goarch)
	}
	return fmt.Sprintf("sortdo_%s_%s.tar.gz", goos, goarch)
}

func download(url string) ([]byte, error) {
	response, err := downloads.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered HTTP %d", url, response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 200<<20))
}

// checksum finds the SHA-256 of a file in checksums.txt ("<hex>  <name>" per line).
func checksum(sums []byte, name string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

// extract returns the sortdo program from a release archive.
func extract(archive []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if path.Base(file.Name) == "sortdo.exe" {
				opened, err := file.Open()
				if err != nil {
					return nil, err
				}
				defer opened.Close()
				return io.ReadAll(io.LimitReader(opened, 200<<20))
			}
		}
		return nil, errors.New("the archive has no sortdo.exe")
	}
	unzipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil, errors.New("the archive has no sortdo program")
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg && path.Base(header.Name) == "sortdo" {
			return io.ReadAll(io.LimitReader(reader, 200<<20))
		}
	}
}

// Fetch downloads the release of the version for this system, checks its SHA-256 and returns the program.
func Fetch(version, goos, goarch string) ([]byte, error) {
	name := AssetName(goos, goarch)
	base := fmt.Sprintf("%s/releases/download/v%s/", Repository(), version)
	sums, err := download(base + "checksums.txt")
	if err != nil {
		return nil, err
	}
	want, err := checksum(sums, name)
	if err != nil {
		return nil, err
	}
	archive, err := download(base + name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("the download of %s does not match its checksum; nothing was changed", name)
	}
	return extract(archive, name)
}

// Replace puts the new program in place of executable. On Windows the running program cannot be overwritten, so it
// moves aside to <name>.old first; RemoveOld deletes it on the next start.
func Replace(executable string, program []byte) error {
	dir := filepath.Dir(executable)
	tmp, err := os.CreateTemp(dir, ".sortdo-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(program); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		old := executable + ".old"
		_ = os.Remove(old)
		if err := os.Rename(executable, old); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), executable); err != nil {
			_ = os.Rename(old, executable)
			return err
		}
		return nil
	}
	return os.Rename(tmp.Name(), executable)
}

// RemoveOld deletes the program a Windows update moved aside.
func RemoveOld(executable string) { _ = os.Remove(executable + ".old") }
