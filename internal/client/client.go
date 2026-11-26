// Package client is a small typed HTTP client for the UpSnap (PocketBase) API.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	SuperusersCollection = "_superusers"
	UsersCollection      = "users"

	defaultTimeout = 5 * time.Minute
	maxAttempts    = 3
)

type Config struct {
	Endpoint           string
	Username           string
	Password           string
	Token              string
	AuthCollection     string
	InsecureSkipVerify bool
	UserAgent          string
	HTTPClient         *http.Client
	// SettleTime is how long Settle watches a written record; zero disables it.
	SettleTime time.Duration
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	settleTime time.Duration

	authCollection string
	username       string
	password       string

	mu    sync.Mutex
	token string
	claim tokenClaims

	Devices         Collection[Device]
	Ports           Collection[Port]
	Groups          Collection[Group]
	Users           Collection[User]
	Permissions     Collection[Permission]
	SettingsPrivate Collection[SettingsPrivate]
	SettingsPublic  Collection[SettingsPublic]
}

func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", cfg.Endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid endpoint %q: scheme must be http or https", cfg.Endpoint)
	}
	if cfg.Token == "" && (cfg.Username == "" || cfg.Password == "") {
		return nil, errors.New("either token or username and password are required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport := &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   true,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		if cfg.InsecureSkipVerify {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in by the user
		}
		httpClient = &http.Client{Transport: transport, Timeout: defaultTimeout}
	}

	authCollection := cfg.AuthCollection
	if authCollection == "" {
		authCollection = SuperusersCollection
	}

	c := &Client{
		baseURL:        u,
		httpClient:     httpClient,
		userAgent:      cfg.UserAgent,
		settleTime:     cfg.SettleTime,
		authCollection: authCollection,
		username:       cfg.Username,
		password:       cfg.Password,
	}
	if cfg.Token != "" {
		claim, err := parseToken(cfg.Token)
		if err != nil {
			return nil, err
		}
		c.token, c.claim = cfg.Token, claim
	}

	c.Devices = Collection[Device]{c: c, name: "devices"}
	c.Ports = Collection[Port]{c: c, name: "ports"}
	c.Groups = Collection[Group]{c: c, name: "device_groups"}
	c.Users = Collection[User]{c: c, name: UsersCollection}
	c.Permissions = Collection[Permission]{c: c, name: "permissions"}
	c.SettingsPrivate = Collection[SettingsPrivate]{c: c, name: "settings_private"}
	c.SettingsPublic = Collection[SettingsPublic]{c: c, name: "settings_public"}
	return c, nil
}

// IsSuperuser reports whether the client is authenticated as a PocketBase superuser.
func (c *Client) IsSuperuser(ctx context.Context) (bool, error) {
	if _, err := c.authToken(ctx); err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claim.CollectionID == superusersCollectionID, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.doWithAuth(ctx, method, path, query, body, out, true)
}

func (c *Client) doWithAuth(ctx context.Context, method, path string, query url.Values, body, out any, authenticated bool) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	reauthenticated := false
	for attempt := 1; ; attempt++ {
		token := ""
		if authenticated {
			var err error
			if token, err = c.authToken(ctx); err != nil {
				return err
			}
		}

		status, respBody, err := c.send(ctx, method, path, query, payload, token)
		if err != nil {
			if ctx.Err() != nil || attempt >= maxAttempts {
				return err
			}
			if !sleep(ctx, backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}

		if status == http.StatusUnauthorized && authenticated && !reauthenticated && c.password != "" {
			reauthenticated = true
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			continue
		}
		if retryable(status) && attempt < maxAttempts {
			if !sleep(ctx, backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}
		if status >= 300 {
			return newAPIError(method, path, status, respBody)
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decoding response of %s %s: %w", method, path, err)
			}
		}
		return nil
	}
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, payload []byte, token string) (int, []byte, error) {
	u := c.baseURL.JoinPath(path)
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("reading response of %s %s: %w", method, path, err)
	}
	return resp.StatusCode, respBody, nil
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt*attempt) * 250 * time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
