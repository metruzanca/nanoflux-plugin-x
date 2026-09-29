package x

import (
	"strings"

	"github.com/metruzanca/nanoflux/pluginapi"
)

// Config carries the logged-in X session used to fetch profile pages. It is set
// on the plugin by the host through Configure (the admin edits it in the app),
// not read from a file or environment.
type Config struct {
	// AuthToken is the value of the `auth_token` cookie (the session).
	AuthToken string
	// Ct0 is the value of the `ct0` cookie (the CSRF token).
	Ct0 string
	// UserAgent optionally overrides the plugin's default browser User-Agent.
	UserAgent string
}

// Complete reports whether the minimum session credentials are present.
func (c Config) Complete() bool {
	return strings.TrimSpace(c.AuthToken) != "" && strings.TrimSpace(c.Ct0) != ""
}

// Cookie renders the Cookie header value for the session.
func (c Config) Cookie() string {
	return "auth_token=" + strings.TrimSpace(c.AuthToken) + "; ct0=" + strings.TrimSpace(c.Ct0)
}

// Settings declares the session the plugin needs. The cookies are write-only so
// the app never renders the stored session back.
func (*Plugin) Settings() []pluginapi.SettingField {
	return []pluginapi.SettingField{
		{
			Name:  "auth_token",
			Label: "auth_token cookie",
			Kind:  "password",
			Help:  "From a logged-in x.com session: DevTools → Application → Cookies → https://x.com.",
		},
		{
			Name:     "ct0",
			Label:    "ct0 cookie",
			Kind:     "password",
			Help:     "The session's CSRF token, from the same cookies list.",
			Required: true,
		},
		{
			Name:        "user_agent",
			Label:       "user agent",
			Kind:        "text",
			Placeholder: defaultUserAgent,
			Help:        "Optional. Override the browser User-Agent the plugin sends.",
		},
	}
}

// Configure caches the session the host pushed.
func (p *Plugin) Configure(values map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = Config{
		AuthToken: values["auth_token"],
		Ct0:       values["ct0"],
		UserAgent: values["user_agent"],
	}
}
