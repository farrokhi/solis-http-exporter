package inverter

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// The real record is under 100 bytes; anything larger is not from a logger.
const maxResponseBytes = 4 << 10

// Options describes one logger endpoint.
type Options struct {
	Scheme   string
	Address  string
	Port     int
	Path     string
	Username string
	Password string
	Timeout  time.Duration
}

// Client fetches readings from a single logger.
type Client struct {
	url      string
	username string
	password string
	http     *http.Client
}

// New builds a client for one logger. The URL is fixed at construction, so
// nothing at scrape time can redirect the request elsewhere.
func New(o Options) *Client {
	u := url.URL{
		Scheme: o.Scheme,
		Host:   net.JoinHostPort(o.Address, strconv.Itoa(o.Port)),
		Path:   o.Path,
	}

	return &Client{
		url:      u.String(),
		username: o.Username,
		password: o.Password,
		http: &http.Client{
			Timeout: o.Timeout,
			// These loggers handle persistent connections poorly.
			Transport: &http.Transport{DisableKeepAlives: true},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// URL returns the endpoint this client reads, for logging.
func (c *Client) URL() string { return c.url }

// Fetch reads one status record.
func (c *Client) Fetch(ctx context.Context) (Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return Status{}, err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.http.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Status{}, err
	}
	if len(body) > maxResponseBytes {
		return Status{}, fmt.Errorf("response larger than %d bytes", maxResponseBytes)
	}

	return Parse(body)
}
