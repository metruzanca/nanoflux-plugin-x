// Package x is a private nanoflux plugin for X (Twitter) profile feeds. X
// offers no public RSS or guest API and serves logged-out profile pages without
// their posts (and hard-walls sensitive accounts), so this plugin fetches the
// profile page with a logged-in session (auth_token/ct0 cookies) and extracts
// the posts embedded in the page payload. The payload format is unofficial;
// treat it as best-effort and expect occasional maintenance.
package x

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metruzanca/nanoflux/pluginapi"
)

// Name is the plugin's stable identifier (stored in feeds.plugin_name). It is
// deliberately "x" so existing feeds owned by the former native plugin are
// adopted without migration.
const Name = "x"

// readme is the plugin's Markdown documentation, shown in the app's docs modal.
//
//go:embed readme.md
var readme string

// defaultUserAgent is a desktop Firefox identity. X serves a page shaped for a
// real browser (and is less likely to flag the session) when a browser UA is
// sent; the host sends this for every mediated request in this plugin.
const defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"

// twitterEpoch is the millisecond epoch Twitter's snowflake IDs are offset from.
const twitterEpoch = 1288834974657 // 2010-11-04T01:42:54Z

// hosts lists hosts treated as X profile pages. It is a var so tests can inject
// a mock host.
var hosts = []string{"x.com", "www.x.com", "twitter.com", "www.twitter.com"}

// Plugin is the X Fetcher. It caches the loaded session config; nanoflux keeps
// one instance alive for the life of the subprocess.
type Plugin struct {
	once   sync.Once
	cfg    Config
	cfgErr error
}

var _ pluginapi.Fetcher = (*Plugin)(nil)

// Meta declares the plugin and its browser User-Agent. The User-Agent may be
// overridden by the session config; the config is loaded once, lazily.
func (p *Plugin) Meta() pluginapi.Meta {
	c := p.loadConfig()
	ua := strings.TrimSpace(c.UserAgent)
	if ua == "" {
		ua = defaultUserAgent
	}
	return pluginapi.Meta{
		Name:       Name,
		APIVersion: pluginapi.APIVersion,
		UserAgent:  ua,
		Summary:    "X profiles: reads a profile with your own logged-in session",
	}
}

// Docs returns this plugin's Markdown documentation.
func (*Plugin) Docs() string { return readme }

func (p *Plugin) loadConfig() Config {
	p.once.Do(func() {
		p.cfg, p.cfgErr = LoadConfig()
	})
	return p.cfg
}

// Match handles both discover and fetch for X profile URLs.
func (p *Plugin) Match(u *url.URL, _ pluginapi.Capability) bool {
	return isProfileURL(u)
}

// Discover: the profile URL is itself the feed URL. When a session is available
// the profile is read so the add form shows the real display name and avatar;
// otherwise the handle is offered as a fallback.
func (p *Plugin) Discover(ctx context.Context, pageURL string, h pluginapi.Host) ([]pluginapi.Candidate, error) {
	u, err := url.Parse(pageURL)
	if err != nil || !isProfileURL(u) {
		return nil, pluginapi.ErrUnsupportedCapability
	}
	handle := handleFromURL(u)
	cand := pluginapi.Candidate{FeedURL: pageURL, Title: handle, HomeURL: pageURL}

	if cfg := p.loadConfig(); cfg.Complete() {
		if body, err := p.get(ctx, pageURL, cfg, h); err == nil {
			cand.Title = displayName(body, handle)
			cand.IconURL = ogImage(body)
		}
	}
	return []pluginapi.Candidate{cand}, nil
}

// Fetch scrapes the profile page with the configured session and extracts its
// embedded recent posts.
func (p *Plugin) Fetch(ctx context.Context, req pluginapi.FetchRequest, h pluginapi.Host) (pluginapi.Result, error) {
	cfg := p.loadConfig()
	if p.cfgErr != nil {
		return pluginapi.Result{}, p.cfgErr
	}
	if !cfg.Complete() {
		return pluginapi.Result{}, errors.New("x session not configured: set auth_token and ct0 (see README.local.md)")
	}

	resp, err := h.Do(ctx, pluginapi.HTTPRequest{
		Method: "GET",
		URL:    req.URL,
		Headers: map[string]string{
			"Cookie":          cfg.Cookie(),
			"x-csrf-token":    cfg.Ct0,
			"Accept":          "text/html,application/xhtml+xml",
			"Accept-Language": "en-US,en;q=0.9",
		},
	})
	if err != nil {
		return pluginapi.Result{}, err
	}
	if resp.RateLimited {
		return pluginapi.Result{}, &pluginapi.RateLimit{URL: req.URL, Status: resp.Status, RetryAfter: resp.RetryAfter}
	}
	if resp.Status >= 400 {
		return pluginapi.Result{}, &pluginapi.StatusError{Code: resp.Status, URL: req.URL}
	}

	u, _ := url.Parse(req.URL)
	handle := handleFromURL(u)
	tweets := tweetsFromPage(resp.Body, handle)
	if len(tweets) == 0 {
		return pluginapi.Result{}, emptyPageError(resp.Body)
	}
	return pluginapi.Result{
		Feed:  pluginapi.Feed{Title: displayName(resp.Body, handle), HomeURL: req.URL, ImageURL: ogImage(resp.Body)},
		Items: tweets,
	}, nil
}

// get performs a session-authenticated GET through the host (used by Discover's
// metadata lookup, where a failed read is non-fatal).
func (p *Plugin) get(ctx context.Context, pageURL string, cfg Config, h pluginapi.Host) ([]byte, error) {
	resp, err := h.Do(ctx, pluginapi.HTTPRequest{
		Method:  "GET",
		URL:     pageURL,
		Headers: map[string]string{"Cookie": cfg.Cookie(), "x-csrf-token": cfg.Ct0},
	})
	if err != nil {
		return nil, err
	}
	if resp.Status >= 400 || resp.RateLimited {
		return nil, &pluginapi.StatusError{Code: resp.Status, URL: pageURL}
	}
	return resp.Body, nil
}

// emptyPageError explains an empty extraction: an account with no posts, a page
// the session still cannot see, or a payload whose shape changed under us.
func emptyPageError(body []byte) error {
	switch {
	case bytes.Contains(body, []byte("hasn’t posted")) || bytes.Contains(body, []byte("hasn't posted")):
		return errors.New("profile has no posts (or is visible only to a logged-in session)")
	case entryIDRe.Match(body) || bytes.Contains(body, []byte("__isTweetResult")):
		return errors.New("x page format changed; the extractor needs updating")
	default:
		return errors.New("no posts found on profile page (session may be invalid or expired)")
	}
}

func isHost(host string) bool {
	host = strings.ToLower(host)
	for _, h := range hosts {
		if host == h {
			return true
		}
	}
	return false
}

// isProfileURL reports whether u is an X profile page (x.com/<handle>), not a
// status, search, home, or other page.
func isProfileURL(u *url.URL) bool {
	if u == nil || !isHost(u.Hostname()) {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		return false
	}
	if strings.TrimPrefix(parts[0], "@") == "" {
		return false
	}
	switch parts[0] {
	case "home", "explore", "search", "notifications", "messages", "settings",
		"login", "signup", "logout", "i", "intent", "hashtag", "moments",
		"share", "compose", "tos", "privacy", "manifest.json":
		return false
	}
	return true
}

func handleFromURL(u *url.URL) string {
	return strings.TrimPrefix(strings.Trim(strings.Trim(u.Path, "/"), "/"), "@")
}

var (
	titleRe   = regexp.MustCompile(`<title>([^<]*)</title>`)
	nameRe    = regexp.MustCompile(`^(.*?)\s*\(@[^)]*\)\s*(?:on X|/ X)`)
	ogImgRe   = regexp.MustCompile(`<meta[^>]+property="og:image"[^>]+content="([^"]+)"`)
	entryIDRe = regexp.MustCompile(`entry_id:"tweet-(\d+)"`)

	// anchorRe finds a tweet result block and captures its snowflake id. It
	// anchors on tweet_results (the tweet itself) rather than any full_text, so
	// a quoted tweet nested inside a post cannot steal the outer post's id.
	// Order of matches is timeline order.
	anchorRe = regexp.MustCompile(`tweet_results:\$R\[\d+\]=\{id:"[^"]*",rest_id:"(\d{15,})",result:\$R\[\d+\]=\{__isTweetResult:"Tweet`)
	// fullTextRe captures the text within one anchor's chunk.
	fullTextRe = regexp.MustCompile(`full_text:"((?:[^"\\]|\\.)*)"`)
)

// displayName extracts the profile's display name from the page <title>
// (e.g. "Sam Altman (@sama) / X" -> "Sam Altman"), falling back to the handle.
func displayName(body []byte, handle string) string {
	if m := titleRe.FindSubmatch(body); len(m) == 2 {
		if name := nameRe.FindSubmatch(m[1]); len(name) == 2 {
			return string(name[1])
		}
		return string(m[1])
	}
	return handle
}

// ogImage extracts the profile avatar from the page's og:image meta tag, or "".
func ogImage(body []byte) string {
	if m := ogImgRe.FindSubmatch(body); len(m) == 2 {
		return string(m[1])
	}
	return ""
}

// tweetsFromPage extracts the posts embedded in the page, ordered as the
// timeline shows them. The display order and the set of shown posts come from
// the timeline's entry ids (`entry_id:"tweet-<id>"`); a quoted tweet nested
// inside a post has a tweet_results block but no entry of its own, so it is not
// emitted as a separate item. Each post's text is the full_text inside its own
// tweet_results result, searched only up to the next anchor so a nested quote's
// text cannot bleed into the outer post.
func tweetsFromPage(body []byte, handle string) []pluginapi.Item {
	text := textByID(body)

	seen := map[string]bool{}
	var ids []string
	for _, m := range entryIDRe.FindAllSubmatch(body, -1) {
		id := string(m[1])
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	// Fallback: a payload without timeline entry ids still has tweet results;
	// use them in anchor order.
	if len(ids) == 0 {
		for _, m := range anchorRe.FindAllSubmatch(body, -1) {
			id := string(m[1])
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}

	out := make([]pluginapi.Item, 0, len(ids))
	for _, id := range ids {
		t := text[id]
		out = append(out, pluginapi.Item{
			GUID:        "tweet:" + id,
			Identity:    "tweet:" + id,
			Title:       PostTitle(t),
			Link:        "https://x.com/" + handle + "/status/" + id,
			Summary:     t,
			PublishedAt: formatTime(snowflakeTime(id)),
		})
	}
	return out
}

// textByID maps a tweet id to its full_text by walking each tweet_results
// anchor and searching only up to the next anchor.
func textByID(body []byte) map[string]string {
	locs := anchorRe.FindAllSubmatchIndex(body, -1)
	out := make(map[string]string, len(locs))
	for i, l := range locs {
		id := string(body[l[2]:l[3]])
		if _, ok := out[id]; ok {
			continue
		}
		end := l[1] + 9000
		if i+1 < len(locs) && locs[i+1][1] < end {
			end = locs[i+1][1]
		}
		if end > len(body) {
			end = len(body)
		}
		text := ""
		if m := fullTextRe.FindSubmatch(body[l[1]:end]); m != nil {
			text = string(m[1])
			if unq, err := strconv.Unquote(`"` + text + `"`); err == nil {
				text = unq
			}
		}
		out[id] = text
	}
	return out
}

// PostTitle derives a list title from a post: the first line, truncated.
func PostTitle(text string) string {
	title := text
	if i := strings.IndexByte(title, '\n'); i >= 0 {
		title = title[:i]
	}
	title = strings.TrimSpace(title)
	if len([]rune(title)) > 100 {
		title = string([]rune(title)[:99]) + "…"
	}
	return title
}

// snowflakeTime recovers a tweet's publish time from its snowflake ID.
func snowflakeTime(id string) time.Time {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli((n >> 22) + twitterEpoch).UTC()
}

const timeFormat = "2006-01-02 15:04:05"

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeFormat)
}
