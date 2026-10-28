// Package store keeps sortdo's state on disk and in the OS keychain.
//
// config.json holds nothing secret: where each server's tokens are. The tokens themselves are in the OS keychain
// (service "sortdo", account = the server origin), or in credentials.json when there is no keychain or
// SORTDO_KEYRING=0. pending-login.json holds a device code that `sortdo login poll` finishes. Every file is written
// to a temporary file and renamed, with mode 0600 in a 0700 folder.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/zalando/go-keyring"
)

const keyringService = "sortdo"

// Tokens are one server's OAuth tokens.
type Tokens struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// Pending is a device sign-in that waits for the person to allow it.
type Pending struct {
	ClientID   string    `json:"clientId"`
	DeviceCode string    `json:"deviceCode"`
	UserCode   string    `json:"userCode"`
	Link       string    `json:"link"`
	Interval   int       `json:"interval"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type serverConfig struct {
	Storage string `json:"storage,omitempty"` // "keychain" or "file"
}

type configFile struct {
	Servers map[string]*serverConfig `json:"servers"`
}

type credentialsFile struct {
	Servers map[string]Tokens `json:"servers"`
}

type pendingFile struct {
	Servers map[string]Pending `json:"servers"`
}

// Dir is $SORTDO_CONFIG_DIR, or ~/.config/sortdo.
func Dir() string {
	if dir := os.Getenv("SORTDO_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".sortdo"
	}
	return filepath.Join(home, ".config", "sortdo")
}

// CredentialsPath is the file that holds tokens when there is no keychain.
func CredentialsPath() string { return filepath.Join(Dir(), "credentials.json") }

func readJSON(name string, value any) {
	data, err := os.ReadFile(filepath.Join(Dir(), name))
	if err == nil {
		_ = json.Unmarshal(data, value)
	}
}

func writeJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(Dir(), name), data)
}

// WriteFile writes data to path through a temporary file and a rename, with mode 0600 in a 0700 folder.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func loadConfig() *configFile {
	cfg := &configFile{}
	readJSON("config.json", cfg)
	if cfg.Servers == nil {
		cfg.Servers = map[string]*serverConfig{}
	}
	return cfg
}

func (cfg *configFile) server(origin string) *serverConfig {
	if cfg.Servers[origin] == nil {
		cfg.Servers[origin] = &serverConfig{}
	}
	return cfg.Servers[origin]
}

func keyringDisabled() bool { return os.Getenv("SORTDO_KEYRING") == "0" }

func loadCredentialsFile() *credentialsFile {
	file := &credentialsFile{}
	readJSON("credentials.json", file)
	if file.Servers == nil {
		file.Servers = map[string]Tokens{}
	}
	return file
}

// SaveTokens keeps the tokens in the keychain, or in credentials.json when there is none. It returns where they went
// and, when the keychain was expected but failed, why.
func SaveTokens(origin string, value Tokens) (place string, keychainError error, err error) {
	cfg := loadConfig()
	entry := cfg.server(origin)
	data, _ := json.Marshal(value)
	if !keyringDisabled() {
		keychainError = keyring.Set(keyringService, origin, string(data))
		if keychainError == nil {
			entry.Storage = "keychain"
			removeFileTokens(origin)
			return "the OS keychain", nil, writeJSON("config.json", cfg)
		}
	}
	file := loadCredentialsFile()
	file.Servers[origin] = value
	if err := writeJSON("credentials.json", file); err != nil {
		return "", keychainError, err
	}
	entry.Storage = "file"
	return CredentialsPath(), keychainError, writeJSON("config.json", cfg)
}

// LoadTokens returns the stored tokens and where they came from, or nil when there is no sign-in for the server.
func LoadTokens(origin string) (*Tokens, string) {
	entry := loadConfig().Servers[origin]
	if entry == nil {
		return nil, ""
	}
	switch entry.Storage {
	case "keychain":
		secret, err := keyring.Get(keyringService, origin)
		if err != nil {
			return nil, ""
		}
		var value Tokens
		if json.Unmarshal([]byte(secret), &value) != nil {
			return nil, ""
		}
		return &value, "OS keychain"
	case "file":
		if value, ok := loadCredentialsFile().Servers[origin]; ok {
			return &value, CredentialsPath()
		}
	}
	return nil, ""
}

func removeFileTokens(origin string) {
	file := loadCredentialsFile()
	if _, ok := file.Servers[origin]; ok {
		delete(file.Servers, origin)
		_ = writeJSON("credentials.json", file)
	}
}

// DeleteTokens removes the server's sign-in from the keychain and the files.
func DeleteTokens(origin string) {
	cfg := loadConfig()
	if entry := cfg.Servers[origin]; entry != nil {
		if entry.Storage == "keychain" {
			_ = keyring.Delete(keyringService, origin)
		}
		delete(cfg.Servers, origin)
		_ = writeJSON("config.json", cfg)
	}
	removeFileTokens(origin)
}

// SignedInServers lists the servers that have a stored sign-in.
func SignedInServers() []string {
	var servers []string
	for origin := range loadConfig().Servers {
		servers = append(servers, origin)
	}
	for origin := range loadCredentialsFile().Servers {
		if !slices.Contains(servers, origin) {
			servers = append(servers, origin)
		}
	}
	slices.Sort(servers)
	return servers
}

// RemoveAll deletes sortdo's files: settings, sign-ins kept in a file, waiting sign-ins and the cache. The folder
// goes too when nothing else is in it.
func RemoveAll() error {
	for _, name := range []string{"config.json", "credentials.json", "pending-login.json"} {
		if err := os.Remove(filepath.Join(Dir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(Dir(), "cache")); err != nil {
		return err
	}
	_ = os.Remove(Dir())
	return nil
}

// LoadPending returns the server's waiting sign-in.
func LoadPending(origin string) (*Pending, bool) {
	file := &pendingFile{}
	readJSON("pending-login.json", file)
	value, ok := file.Servers[origin]
	return &value, ok
}

// SavePending stores the waiting sign-in, or removes it when value is nil.
func SavePending(origin string, value *Pending) error {
	file := &pendingFile{}
	readJSON("pending-login.json", file)
	if file.Servers == nil {
		file.Servers = map[string]Pending{}
	}
	if value == nil {
		if _, ok := file.Servers[origin]; !ok {
			return nil
		}
		delete(file.Servers, origin)
	} else {
		file.Servers[origin] = *value
	}
	return writeJSON("pending-login.json", file)
}

func cachePath(name, origin string) string {
	sum := sha256.Sum256([]byte(origin))
	return filepath.Join(Dir(), "cache", name+"-"+hex.EncodeToString(sum[:6])+".json")
}

// ReadCache returns a cached value for the server and when it was saved.
func ReadCache(name, origin string, value any) (time.Time, bool) {
	var wrapper struct {
		SavedAt time.Time       `json:"savedAt"`
		Value   json.RawMessage `json:"value"`
	}
	data, err := os.ReadFile(cachePath(name, origin))
	if err != nil || json.Unmarshal(data, &wrapper) != nil || json.Unmarshal(wrapper.Value, value) != nil {
		return time.Time{}, false
	}
	return wrapper.SavedAt, true
}

// WriteCache saves a value for the server. Failures are ignored: a cache only saves requests.
func WriteCache(name, origin string, value any) {
	data, err := json.Marshal(map[string]any{"savedAt": time.Now().UTC(), "value": value})
	if err == nil {
		_ = WriteFile(cachePath(name, origin), data)
	}
}

// ClearCache removes the server's cached value.
func ClearCache(name, origin string) { _ = os.Remove(cachePath(name, origin)) }
