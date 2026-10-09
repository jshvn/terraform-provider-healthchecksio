package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testKey = "abcdefghijklmnopqrstuvwxyz012345"

// newTest gives a client for a test server with handler h. The client records each
// wait and does not sleep.
func newTest(t *testing.T, h http.HandlerFunc) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/api/v3", testKey, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	return c, &waits
}

// The API reference gives this UUID and unique_key for one check.
func TestUniqueKey(t *testing.T) {
	got := UniqueKey("31365bce-8da9-4729-8ff3-aaa71d56b712")
	if want := "a6c7b0a8a66bed0df66abfdab3c77736861703ee"; got != want {
		t.Fatalf("UniqueKey = %s, want %s", got, want)
	}
	if !IsUniqueKey(got) || IsUniqueKey("31365bce-8da9-4729-8ff3-aaa71d56b712") {
		t.Fatal("IsUniqueKey does not tell a unique_key from a UUID")
	}
}

// An empty string goes into the body, and a nil field does not.
func TestUpdateSendsSpec(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/checks/u-1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != testKey || r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("headers = %v", r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		_ = json.Unmarshal(body, &got)
		if len(got) != 2 || got["channels"] != "" || got["grace"] != float64(60) {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"uuid": "u-1", "schedule": "* * * * *", "tz": "UTC"}`))
	})
	empty, grace := "", int64(60)
	got, err := c.UpdateCheck(context.Background(), "u-1", CheckSpec{Channels: &empty, Grace: &grace})
	if err != nil || got.UUID != "u-1" || *got.Schedule != "* * * * *" || got.Timeout != nil {
		t.Fatalf("UpdateCheck = %+v, %v", got, err)
	}
}

// After HTTP 429, the client tries again. After a 5xx for a create, it does not.
func TestRetries(t *testing.T) {
	calls := 0
	c, waits := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case r.Method == http.MethodGet && calls < 3:
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"checks": [{"uuid": "u-1"}]}`))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	checks, err := c.ListChecks(context.Background(), CheckFilter{Tags: []string{"a", "b"}})
	if err != nil || len(checks) != 1 || len(*waits) != 2 || (*waits)[0] != 7*time.Second {
		t.Fatalf("ListChecks = %v, %v, waits %v", checks, err, *waits)
	}
	calls = 0
	if _, err := c.CreateCheck(context.Background(), CheckSpec{}); err == nil || calls != 1 {
		t.Fatalf("CreateCheck after 503: err %v, calls %d", err, calls)
	}
}

// A network error does not show the URL, because the URL can contain the UUID. The
// client tries again after a network error, but not for a create.
func TestNetworkError(t *testing.T) {
	c, waits := newTest(t, func(http.ResponseWriter, *http.Request) {})
	c.base, _ = url.Parse("http://127.0.0.1:1/api/v3/") // No server listens on port 1.
	_, err := c.GetCheck(context.Background(), "31365bce-8da9-4729-8ff3-aaa71d56b712")
	if err == nil || strings.Contains(err.Error(), "31365bce") || len(*waits) != maxAttempts-1 {
		t.Fatalf("GetCheck: err %v, waits %v", err, *waits)
	}
	*waits = nil
	if _, err := c.CreateCheck(context.Background(), CheckSpec{}); err == nil || len(*waits) != 0 {
		t.Fatalf("CreateCheck: err %v, waits %v", err, *waits)
	}
}

// The message has no path: a path can contain the UUID, which is the ping credential,
// and Terraform shows the message.
func TestNotFound(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	})
	_, err := c.GetCheck(context.Background(), "u-1")
	if !IsNotFound(err) || err.Error() != "healthchecks API GET: 404 not found" {
		t.Fatalf("GetCheck = %v", err)
	}
}

func TestNewRejectsOldVersions(t *testing.T) {
	for _, u := range []string{"https://healthchecks.io/api/v1/", "healthchecks.io/api/v3", "ftp://x/api/v3"} {
		if _, err := New(u, testKey, ""); err == nil {
			t.Errorf("New(%q) accepted it", u)
		}
	}
	if _, err := New(DefaultURL, "short", ""); err == nil {
		t.Error("New accepted a short key")
	}
}
