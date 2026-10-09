package client

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: see UniqueKey.
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Check is one check in an API response. With a read-write key, the response has UUID,
// PingURL and Channels. With a read-only key, it has UniqueKey in their place. A simple
// check has Timeout. A cron or OnCalendar check has Schedule and TZ.
type Check struct {
	Name              string  `json:"name"`
	Slug              string  `json:"slug"`
	Tags              string  `json:"tags"`
	Desc              string  `json:"desc"`
	Grace             int64   `json:"grace"`
	NPings            int64   `json:"n_pings"`
	Status            string  `json:"status"`
	Started           bool    `json:"started"`
	LastPing          *string `json:"last_ping"`
	NextPing          *string `json:"next_ping"`
	LastDuration      *int64  `json:"last_duration"`
	ManualResume      bool    `json:"manual_resume"`
	Methods           string  `json:"methods"`
	StartKw           string  `json:"start_kw"`
	SuccessKw         string  `json:"success_kw"`
	FailureKw         string  `json:"failure_kw"`
	FilterSubject     bool    `json:"filter_subject"`
	FilterBody        bool    `json:"filter_body"`
	FilterHTTPBody    bool    `json:"filter_http_body"`
	FilterDefaultFail bool    `json:"filter_default_fail"`
	BadgeURL          string  `json:"badge_url"`
	UUID              string  `json:"uuid"`
	PingURL           string  `json:"ping_url"`
	Channels          string  `json:"channels"`
	UniqueKey         string  `json:"unique_key"`
	Timeout           *int64  `json:"timeout"`
	Schedule          *string `json:"schedule"`
	TZ                *string `json:"tz"`
}

// Key gives the unique_key of the check. A response for a read-write key does not
// contain the unique_key. Thus, Key calculates it from the UUID.
func (c *Check) Key() string {
	if c.UniqueKey != "" {
		return c.UniqueKey
	}
	return UniqueKey(c.UUID)
}

// UniqueKey calculates the read-only identifier of a check from its UUID, with the same
// method as the server (Check.unique_key in hc/api/models.py): the SHA-1 of the first
// 16 hex digits. The unique_key is stable. It is not a ping credential, but the UUID is.
func UniqueKey(uuid string) string {
	h := strings.ToLower(strings.ReplaceAll(uuid, "-", ""))
	if len(h) != 32 {
		return ""
	}
	sum := sha1.Sum([]byte(h[:16])) //nolint:gosec // G401: an identifier that the server makes with SHA-1, not a security control.
	return hex.EncodeToString(sum[:])
}

// IsUniqueKey tells if s has the format of a unique_key: 40 hex digits.
func IsUniqueKey(s string) bool {
	if len(s) != 40 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// CheckSpec is the body of a create or an update. A nil field is not in the body. For a
// create, the server then uses the default value. For an update, the server keeps the
// value. Do not send null: the server rejects it.
type CheckSpec struct {
	Name              *string `json:"name,omitempty"`
	Slug              *string `json:"slug,omitempty"`
	Tags              *string `json:"tags,omitempty"`
	Desc              *string `json:"desc,omitempty"`
	Timeout           *int64  `json:"timeout,omitempty"`
	Grace             *int64  `json:"grace,omitempty"`
	Schedule          *string `json:"schedule,omitempty"`
	TZ                *string `json:"tz,omitempty"`
	ManualResume      *bool   `json:"manual_resume,omitempty"`
	Methods           *string `json:"methods,omitempty"`
	Channels          *string `json:"channels,omitempty"`
	StartKw           *string `json:"start_kw,omitempty"`
	SuccessKw         *string `json:"success_kw,omitempty"`
	FailureKw         *string `json:"failure_kw,omitempty"`
	FilterSubject     *bool   `json:"filter_subject,omitempty"`
	FilterBody        *bool   `json:"filter_body,omitempty"`
	FilterHTTPBody    *bool   `json:"filter_http_body,omitempty"`
	FilterDefaultFail *bool   `json:"filter_default_fail,omitempty"`
}

// CheckFilter selects the checks for ListChecks. A check must have each tag in Tags.
type CheckFilter struct {
	Slug string
	Tags []string
}

// ListChecks gives the checks of the project that agree with f.
func (c *Client) ListChecks(ctx context.Context, f CheckFilter) ([]Check, error) {
	q := url.Values{}
	if f.Slug != "" {
		q.Set("slug", f.Slug)
	}
	for _, t := range f.Tags {
		q.Add("tag", t)
	}
	var out struct {
		Checks []Check `json:"checks"`
	}
	err := c.do(ctx, http.MethodGet, "checks/", q, nil, &out)
	return out.Checks, err
}

// GetCheck gives one check. id is a UUID or a unique_key.
func (c *Client) GetCheck(ctx context.Context, id string) (*Check, error) {
	var out Check
	if err := c.do(ctx, http.MethodGet, "checks/"+id, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCheck makes a check.
func (c *Client) CreateCheck(ctx context.Context, spec CheckSpec) (*Check, error) {
	var out Check
	if err := c.do(ctx, http.MethodPost, "checks/", nil, spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCheck changes the fields of the check that spec contains.
func (c *Client) UpdateCheck(ctx context.Context, uuid string, spec CheckSpec) (*Check, error) {
	var out Check
	if err := c.do(ctx, http.MethodPost, "checks/"+uuid, nil, spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCheck removes the check.
func (c *Client) DeleteCheck(ctx context.Context, uuid string) error {
	return c.do(ctx, http.MethodDelete, "checks/"+uuid, nil, nil, nil)
}

// PauseCheck pauses the check. A paused check sends no alerts. It stays paused until
// the next ping, or until ResumeCheck when manual_resume is true.
func (c *Client) PauseCheck(ctx context.Context, uuid string) (*Check, error) {
	var out Check
	if err := c.do(ctx, http.MethodPost, "checks/"+uuid+"/pause", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResumeCheck resumes a paused check. If the check is not
// paused, the server sends HTTP 409.
func (c *Client) ResumeCheck(ctx context.Context, uuid string) (*Check, error) {
	var out Check
	if err := c.do(ctx, http.MethodPost, "checks/"+uuid+"/resume", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Ping is one logged ping. Type is the kind of the ping, for example success, start,
// fail or log.
type Ping struct {
	Type       string   `json:"type"`
	Date       string   `json:"date"`
	N          int64    `json:"n"`
	Scheme     string   `json:"scheme"`
	RemoteAddr string   `json:"remote_addr"`
	Method     string   `json:"method"`
	UA         string   `json:"ua"`
	RID        *string  `json:"rid"`
	Duration   *float64 `json:"duration"`
	BodyURL    *string  `json:"body_url"`
}

// ListPings gives the logged pings of the check, the newest first. A free account gets
// a maximum of 100 pings, and a paid account gets a maximum of 1000.
func (c *Client) ListPings(ctx context.Context, uuid string) ([]Ping, error) {
	var out struct {
		Pings []Ping `json:"pings"`
	}
	err := c.do(ctx, http.MethodGet, "checks/"+uuid+"/pings/", nil, nil, &out)
	return out.Pings, err
}

// PingBody gives the logged request body of ping n. The body can be binary.
func (c *Client) PingBody(ctx context.Context, uuid string, n int64) ([]byte, error) {
	var out []byte
	err := c.do(ctx, http.MethodGet, "checks/"+uuid+"/pings/"+strconv.FormatInt(n, 10)+"/body", nil, nil, &out)
	return out, err
}

// Flip is one change of status. Up is 1 for a change to "up", and 0 for a change to
// "down".
type Flip struct {
	Timestamp string `json:"timestamp"`
	Up        int64  `json:"up"`
}

// FlipFilter selects the flips for ListFlips. Start and End are UNIX timestamps. A zero
// field is not in the query.
type FlipFilter struct {
	Seconds, Start, End int64
}

// ListFlips gives the flips of the check. id is a UUID or a unique_key. The API
// reference shows an array, but the server puts the array in an object:
// {"flips": [...]} (flips in hc/api/views.py).
func (c *Client) ListFlips(ctx context.Context, id string, f FlipFilter) ([]Flip, error) {
	q := url.Values{}
	for k, v := range map[string]int64{"seconds": f.Seconds, "start": f.Start, "end": f.End} {
		if v != 0 {
			q.Set(k, strconv.FormatInt(v, 10))
		}
	}
	var out struct {
		Flips []Flip `json:"flips"`
	}
	err := c.do(ctx, http.MethodGet, "checks/"+id+"/flips/", q, nil, &out)
	return out.Flips, err
}
