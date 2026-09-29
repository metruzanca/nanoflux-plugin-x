package x

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metruzanca/nanoflux/pluginapi"
)

// hostFunc adapts a function to pluginapi.Host.
type hostFunc func(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error)

func (h hostFunc) Do(ctx context.Context, req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	return h(ctx, req)
}
func (hostFunc) Now() time.Time      { return time.Now().UTC() }
func (hostFunc) Logf(string, ...any) {}

// withConfig primes a plugin with a session so tests can exercise Fetch.
func withConfig(c Config) *Plugin {
	p := &Plugin{}
	p.Configure(map[string]string{
		"auth_token": c.AuthToken,
		"ct0":        c.Ct0,
		"user_agent": c.UserAgent,
	})
	return p
}

func TestIsProfileURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://x.com/sama", true},
		{"https://x.com/sama/", true},
		{"https://www.x.com/sama", true},
		{"https://twitter.com/sama", true},
		{"https://x.com/@sama", true},
		{"https://x.com/home", false},
		{"https://x.com/explore", false},
		{"https://x.com/i/flow/login", false},
		{"https://x.com/sama/status/123", false},
		{"https://example.com/sama", false},
		{"https://x.com", false},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.raw)
		if got := isProfileURL(u); got != c.want {
			t.Errorf("isProfileURL(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestSnowflakeTime(t *testing.T) {
	got := snowflakeTime("2100351958167220547")
	want := time.Date(2026, 9, 16, 22, 31, 44, 136e6, time.UTC)
	if !got.Equal(want) {
		t.Errorf("snowflakeTime = %v, want %v", got, want)
	}
	if !snowflakeTime("notanumber").IsZero() {
		t.Error("invalid id should give zero time")
	}
}

func TestPostTitle(t *testing.T) {
	if got := PostTitle("first line\nsecond line"); got != "first line" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("a", 200)
	if got := PostTitle(long); len([]rune(got)) != 100 || !strings.HasSuffix(got, "…") {
		t.Errorf("long title = %q", got)
	}
}

// tweetBlock renders one tweet result in the current payload shape:
//
//	tweet_results:$R[n]={id:"...",rest_id:"<id>",result:$R[n]={__isTweetResult:"Tweet",...details:$R[n]={...,full_text:"..."}}
func tweetBlock(id, text string) string {
	return fmt.Sprintf(`tweet_results:$R[61]={id:"VHdlZXRSZXN1bHRzOjIxMDM2MTAyODQ0NjQzMTY0Njk=",rest_id:"%s",result:$R[62]={__isTweetResult:"Tweet",core:$R[64]={},details:$R[75]={cashtag_entities:$R[76]=[],created_at_ms:1790374749000,display_text_range:$R[77]=[0,197],full_text:"%s"}},counts:$R[74]={favorite_count:1}}`, id, jsonEscape(text))
}

// fixture builds a profile page for the given handle with the given tweets in
// timeline order.
func fixture(handle string, tweets []struct{ id, text string }) string {
	var b strings.Builder
	b.WriteString(`<html><head><title>Test User (@` + handle + `) / X</title>`)
	b.WriteString(`<meta property="og:image" content="https://pbs.twimg.com/profile_images/1/avatar_normal.jpg"></head><body>`)
	for _, tw := range tweets {
		b.WriteString(tweetBlock(tw.id, tw.text))
		b.WriteString(`entry_id:"tweet-` + tw.id + `",sort_index:"1"`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func jsonEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func session() Config { return Config{AuthToken: "tok", Ct0: "csrf"} }

func TestFetchProfile(t *testing.T) {
	tweets := []struct{ id, text string }{
		{"2100351958167220547", "the main thing i was excited about launching this week will be next week instead"},
		{"2099872600977760451", "big this week\n\nand then for devday"},
		{"2099352016988614852", "There are two ways AI progress could go very badly"},
	}
	h := hostFunc(func(_ context.Context, req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		if !strings.Contains(req.Headers["Cookie"], "auth_token=tok") {
			t.Errorf("request missing session cookie: %+v", req.Headers)
		}
		return pluginapi.HTTPResponse{Status: 200, Body: []byte(fixture("sama", tweets))}, nil
	})
	res, err := withConfig(session()).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/sama"}, h)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Feed.Title != "Test User" || res.Feed.HomeURL != "https://x.com/sama" {
		t.Errorf("feed = %+v", res.Feed)
	}
	if res.Feed.ImageURL == "" {
		t.Error("expected feed avatar from og:image")
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(res.Items))
	}
	first := res.Items[0]
	if first.GUID != "tweet:2100351958167220547" || first.Identity != "tweet:2100351958167220547" {
		t.Errorf("first identity = %+v", first)
	}
	if first.Link != "https://x.com/sama/status/2100351958167220547" {
		t.Errorf("first link = %q", first.Link)
	}
	if first.PublishedAt != "2026-09-16 22:31:44" {
		t.Errorf("published = %q", first.PublishedAt)
	}
	if res.Items[1].Title != "big this week" {
		t.Errorf("second title = %q", res.Items[1].Title)
	}
}

// TestFetchQuoteTweetKeepsOuterID guards the anchor choice: a quoted tweet's
// full_text block follows the outer post, and a naive block scan would attach
// the quoted id/text to the outer entry.
func TestFetchQuoteTweetKeepsOuterID(t *testing.T) {
	outer := "2100351958167220547"
	inner := "2000000000000000000"
	body := `<html><head><title>Quoter (@quoter) / X</title></head><body>` +
		fmt.Sprintf(`tweet_results:$R[61]={id:"outer",rest_id:"%s",result:$R[62]={__isTweetResult:"Tweet",details:$R[75]={full_text:"quoting this"`, outer) +
		fmt.Sprintf(`quoted:tweet_results:$R[90]={id:"inner",rest_id:"%s",result:$R[91]={__isTweetResult:"Tweet",details:$R[92]={full_text:"the quoted post"`, inner) +
		`}}}` +
		fmt.Sprintf(`tweet_results:$R[100]={id:"next",rest_id:"%s",result:$R[101]={__isTweetResult:"Tweet",details:$R[102]={full_text:"a later post"`, "1999999999999999999") +
		`}}}` +
		`entry_id:"tweet-` + outer + `",sort_index:"2"` +
		`entry_id:"tweet-1999999999999999999",sort_index:"1"` +
		`</body></html>`

	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{Status: 200, Body: []byte(body)}, nil
	})
	res, err := withConfig(session()).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/quoter"}, h)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2 (quote must not add an entry): %+v", len(res.Items), res.Items)
	}
	if res.Items[0].GUID != "tweet:"+outer {
		t.Errorf("outer id stolen: got %q", res.Items[0].GUID)
	}
	if res.Items[0].Summary != "quoting this" {
		t.Errorf("outer text = %q, want %q", res.Items[0].Summary, "quoting this")
	}
}

func TestFetchNoPosts(t *testing.T) {
	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{Status: 200, Body: []byte(`<html><title>someone</title></html>`)}, nil
	})
	if _, err := withConfig(session()).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/someone"}, h); err == nil {
		t.Fatal("expected error when no posts are embedded")
	}
}

func TestFetchEmptyTimelineWall(t *testing.T) {
	body := `<html><title>someone</title><div>@someone hasn’t posted</div></html>`
	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{Status: 200, Body: []byte(body)}, nil
	})
	_, err := withConfig(session()).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/someone"}, h)
	if err == nil || !strings.Contains(err.Error(), "no posts") {
		t.Fatalf("expected empty-timeline error, got %v", err)
	}
}

func TestFetchWithoutSession(t *testing.T) {
	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		t.Fatal("must not hit the network without a session")
		return pluginapi.HTTPResponse{}, nil
	})
	_, err := withConfig(Config{}).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/sama"}, h)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured error, got %v", err)
	}
}

func TestFetchRateLimited(t *testing.T) {
	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{Status: 429, RateLimited: true, RetryAfter: time.Minute}, nil
	})
	_, err := withConfig(session()).Fetch(context.Background(), pluginapi.FetchRequest{URL: "https://x.com/sama"}, h)
	var rl *pluginapi.RateLimit
	if !asRateLimit(err, &rl) {
		t.Fatalf("expected RateLimit, got %v", err)
	}
}

func TestDiscoverOffersHandleWithoutSession(t *testing.T) {
	h := hostFunc(func(_ context.Context, _ pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		t.Fatal("Discover must not fetch without a session")
		return pluginapi.HTTPResponse{}, nil
	})
	cs, err := withConfig(Config{}).Discover(context.Background(), "https://x.com/sama", h)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(cs) != 1 || cs[0].Title != "sama" || cs[0].FeedURL != "https://x.com/sama" {
		t.Fatalf("candidates = %+v", cs)
	}
}

func asRateLimit(err error, target **pluginapi.RateLimit) bool {
	if rl, ok := err.(*pluginapi.RateLimit); ok {
		*target = rl
		return true
	}
	return false
}
