package x

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config carries the logged-in X session used to fetch profile pages. It lives
// in a JSON file next to the plugin binary (config.json) or at the path in
// NF_X_CONFIG. Neither file is tracked by nanoflux.
type Config struct {
	// AuthToken is the value of the `auth_token` cookie (the session).
	AuthToken string `json:"auth_token"`
	// Ct0 is the value of the `ct0` cookie (the CSRF token).
	Ct0 string `json:"ct0"`
	// UserAgent optionally overrides the plugin's default browser User-Agent.
	UserAgent string `json:"user_agent,omitempty"`
}

// Complete reports whether the minimum session credentials are present.
func (c Config) Complete() bool {
	return strings.TrimSpace(c.AuthToken) != "" && strings.TrimSpace(c.Ct0) != ""
}

// Cookie renders the Cookie header value for the session.
func (c Config) Cookie() string {
	return "auth_token=" + strings.TrimSpace(c.AuthToken) + "; ct0=" + strings.TrimSpace(c.Ct0)
}

// LoadConfig reads the session config. NF_X_CONFIG names an explicit path;
// otherwise config.json is looked up next to the running executable. A missing
// file is not an error: it returns an empty config so the caller can report the
// session as unconfigured.
func LoadConfig() (Config, error) {
	path, explicit := configPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read x session config %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse x session config %s: %w", path, err)
	}
	return c, nil
}

// configPath returns the config file path and whether it was set explicitly
// (via NF_X_CONFIG) rather than defaulted next to the executable.
func configPath() (string, bool) {
	if v := strings.TrimSpace(os.Getenv("NF_X_CONFIG")); v != "" {
		return v, true
	}
	exe, err := os.Executable()
	if err != nil {
		return "config.json", false
	}
	return filepath.Join(filepath.Dir(exe), "config.json"), false
}
