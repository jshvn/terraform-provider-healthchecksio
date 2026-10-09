package client

import (
	"context"
	"net/http"
)

// Channel is an integration. The API can show the integrations of a project. It cannot
// make or change them.
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ListChannels gives each integration of the project.
func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	var out struct {
		Channels []Channel `json:"channels"`
	}
	err := c.do(ctx, http.MethodGet, "channels/", nil, nil, &out)
	return out.Channels, err
}

// Badges contains the badge URLs of one tag. The 2-state forms show "up" for a check in
// its grace period. The 3-state forms (the names that end in 3) show up, late and down.
type Badges struct {
	SVG      string `json:"svg"`
	SVG3     string `json:"svg3"`
	JSON     string `json:"json"`
	JSON3    string `json:"json3"`
	Shields  string `json:"shields"`
	Shields3 string `json:"shields3"`
}

// ListBadges gives the badges of each tag. The "*" entry is for all checks of the
// project.
func (c *Client) ListBadges(ctx context.Context) (map[string]Badges, error) {
	var out struct {
		Badges map[string]Badges `json:"badges"`
	}
	err := c.do(ctx, http.MethodGet, "badges/", nil, nil, &out)
	return out.Badges, err
}

// Status tells if the server can query its database. The endpoint does not need a key.
func (c *Client) Status(ctx context.Context) error {
	var out []byte
	return c.do(ctx, http.MethodGet, "status/", nil, nil, &out)
}
