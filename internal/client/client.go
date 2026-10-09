// Package client is a client for the healthchecks.io Management API v3. It has a
// function for each documented endpoint. It uses only the Go standard library.
//
// The API reference is at https://healthchecks.io/docs/api/.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the API root of the hosted service. For a self-hosted server, the API
// root is <SITE_ROOT>/api/v3/.
const DefaultURL = "https://healthchecks.io/api/v3/"

// maxAttempts is the maximum number of attempts for one request, with retries after
// HTTP 429 and 5xx. The API accepts a maximum of 100 requests in one minute. The last
// wait (8 s) is longer than a short burst.
const maxAttempts = 5

// Client can operate from more than one goroutine at the same time.
//
// Some servers (for example uWSGI with http-socket) send HTTP/1.1 with no
// "Connection: close", and then close the connection. Go can send the next request on
// that closed connection. Go sends such a request again only if the request is
// idempotent. Thus, send marks each request as idempotent, except a create. A create
// uses fresh, a client that opens a new connection for each request. A create then
// cannot go on a closed connection, and cannot make two checks.
type Client struct {
	// Log gets one entry for each attempt of a request. If it is nil, the client writes
	// no log. The provider sets it to tflog.
	Log Logger
	// Sleep is the wait between two attempts of a request. A test replaces it.
	Sleep func(context.Context, time.Duration) error

	base      *url.URL
	key       string
	userAgent string
	http      *http.Client
	fresh     *http.Client
}

// Logger writes one log entry. The fields contain no API key. The path can contain the
// UUID, which is the ping credential. The provider masks it.
type Logger func(ctx context.Context, msg string, fields map[string]any)

// New gives a client for the API root baseURL. The path of baseURL must end in /api/v3.
// New does not accept v1 or v2. These versions make a slug from the name, and they show
// the "started" status differently. Thus, a plan can show incorrect values.
func New(baseURL, apiKey, userAgent string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("api_url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return nil, fmt.Errorf("api_url %q: want an absolute http(s) URL", baseURL)
	}
	if !strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/api/v3") {
		return nil, fmt.Errorf("api_url %q: want a URL ending in /api/v3/", baseURL)
	}
	if len(apiKey) != 32 {
		// The server rejects a key that does not have 32 characters. It does this
		// before it looks for the key.
		return nil, errors.New("api_key: want a 32-character project API key")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/"
	noReuse := http.DefaultTransport.(*http.Transport).Clone()
	noReuse.DisableKeepAlives = true
	return &Client{
		base:      u,
		key:       apiKey,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
		fresh:     &http.Client{Timeout: 30 * time.Second, Transport: noReuse},
		Sleep:     sleepCtx,
	}, nil
}

// APIError is a response with a status other than 2xx. Message is the "error" field of
// the response body, if the body has one. The error has no path: a path can contain the
// UUID, which is the ping credential, and Terraform shows the error.
type APIError struct {
	Method  string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("healthchecks API %s: %d %s", e.Method, e.Status, msg)
}

// IsNotFound tells if err is an HTTP 404 from the API.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// do sends one request. If out is not nil, do decodes the JSON response into out. If
// out is a *[]byte, do puts the raw body into it. After HTTP 429, do tries again. After
// a 5xx or a network error, do tries again, but not for a create: the server can have
// made the check.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	create := method == http.MethodPost && path == "checks/"
	for attempt := 1; ; attempt++ {
		start := time.Now()
		status, data, retryAfter, err := c.send(ctx, method, path, query, body, create)
		c.log(ctx, method, path, attempt, status, time.Since(start), err)
		retryable := status == http.StatusTooManyRequests || status >= 500 && !create ||
			err != nil && !create && ctx.Err() == nil
		if retryable && attempt < maxAttempts {
			if err := c.Sleep(ctx, backoff(attempt, retryAfter)); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if status/100 != 2 {
			return apiError(method, status, data)
		}
		if out == nil {
			return nil
		}
		if b, ok := out.(*[]byte); ok {
			*b = data
			return nil
		}
		return json.Unmarshal(data, out)
	}
}

// send does one HTTP request. It gives the status, the body and the Retry-After header.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body []byte, create bool) (int, []byte, string, error) {
	u := c.base.JoinPath(path)
	u.RawQuery = query.Encode()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.fresh
	if !create {
		// An empty Idempotency-Key marks the request as idempotent for Go. Go does not
		// send the header.
		req.Header["Idempotency-Key"] = []string{}
		hc = c.http
	}
	res, err := hc.Do(req)
	if err != nil {
		// A *url.Error shows the URL, and the URL can contain the UUID. Terraform shows
		// the error. Thus, send keeps only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, nil, "", fmt.Errorf("healthchecks API %s: %w", method, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	return res.StatusCode, data, res.Header.Get("Retry-After"), err
}

// log writes one entry for an attempt, if Log is set.
func (c *Client) log(ctx context.Context, method, path string, attempt, status int, d time.Duration, err error) {
	if c.Log == nil {
		return
	}
	fields := map[string]any{
		"method":      method,
		"path":        path,
		"attempt":     attempt,
		"status":      status,
		"duration_ms": d.Milliseconds(),
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	c.Log(ctx, "healthchecks API request", fields)
}

func apiError(method string, status int, data []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &e)
	return &APIError{Method: method, Status: status, Message: e.Error}
}

// backoff is the wait before the next attempt. If Retry-After gives a number of seconds
// from 1 to 60, backoff uses it. If not, backoff waits 1, 2, 4 or 8 s.
func backoff(attempt int, retryAfter string) time.Duration {
	if s, err := strconv.Atoi(retryAfter); err == nil && s > 0 && s <= 60 {
		return time.Duration(s) * time.Second
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

// sleepCtx waits for d. It stops early with an error when ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
