package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	superusersCollectionID = "pbc_3142635823"
	refreshBefore          = 5 * time.Minute
)

var nowFunc = time.Now

type tokenClaims struct {
	ID           string `json:"id"`
	CollectionID string `json:"collectionId"`
	Refreshable  bool   `json:"refreshable"`
	Exp          int64  `json:"exp"`
}

func (t tokenClaims) expiresWithin(d time.Duration) bool {
	return t.Exp != 0 && nowFunc().Add(d).Unix() >= t.Exp
}

func parseToken(token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, errors.New("token is not a valid PocketBase JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, fmt.Errorf("token is not a valid PocketBase JWT: %w", err)
	}
	var claims tokenClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return tokenClaims{}, fmt.Errorf("token is not a valid PocketBase JWT: %w", err)
	}
	return claims, nil
}

type authResponse struct {
	Token string `json:"token"`
}

// authToken returns a valid token, logging in or refreshing it when it is missing or about to expire.
func (c *Client) authToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && !c.claim.expiresWithin(refreshBefore) {
		return c.token, nil
	}

	if c.token != "" && c.claim.Refreshable {
		if err := c.refreshLocked(ctx); err == nil {
			return c.token, nil
		} else if c.password == "" {
			return "", err
		}
	}

	if c.password == "" {
		if c.token != "" && !c.claim.expiresWithin(0) {
			return c.token, nil
		}
		return "", &AuthError{Message: "the configured token has expired and cannot be refreshed; configure username and password or provide a new token"}
	}
	if err := c.loginLocked(ctx); err != nil {
		return "", err
	}
	return c.token, nil
}

func (c *Client) loginLocked(ctx context.Context) error {
	var resp authResponse
	path := "/api/collections/" + c.authCollection + "/auth-with-password"
	err := c.doWithAuth(ctx, http.MethodPost, path, nil, map[string]string{
		"identity": c.username,
		"password": c.password,
	}, &resp, false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
			return &AuthError{Message: fmt.Sprintf("authenticating %q against the %s collection failed: %s", c.username, c.authCollection, apiErr.Message), Err: err}
		}
		return err
	}
	return c.setTokenLocked(resp.Token)
}

func (c *Client) refreshLocked(ctx context.Context) error {
	collection := c.claim.CollectionID
	if collection == "" {
		collection = c.authCollection
	}
	var resp authResponse
	err := c.sendAuthenticated(ctx, http.MethodPost, "/api/collections/"+collection+"/auth-refresh", c.token, &resp)
	if err != nil {
		return &AuthError{Message: "refreshing the auth token failed", Err: err}
	}
	return c.setTokenLocked(resp.Token)
}

func (c *Client) sendAuthenticated(ctx context.Context, method, path, token string, out any) error {
	status, body, err := c.send(ctx, method, path, nil, nil, token)
	if err != nil {
		return err
	}
	if status >= 300 {
		return newAPIError(method, path, status, body)
	}
	return json.Unmarshal(body, out)
}

func (c *Client) setTokenLocked(token string) error {
	if token == "" {
		return &AuthError{Message: "the auth response did not contain a token"}
	}
	claims, err := parseToken(token)
	if err != nil {
		return err
	}
	c.token, c.claim = token, claims
	return nil
}

// Token returns the current auth token, logging in or refreshing it first when needed.
func (c *Client) Token(ctx context.Context) (string, error) {
	return c.authToken(ctx)
}

// Authenticate obtains or verifies a token now so that bad credentials fail fast.
func (c *Client) Authenticate(ctx context.Context) error {
	if _, err := c.authToken(ctx); err != nil {
		return err
	}
	if c.password != "" {
		return nil
	}
	c.mu.Lock()
	claim, token := c.claim, c.token
	c.mu.Unlock()
	if claim.ID == "" || claim.CollectionID == "" {
		return &AuthError{Message: "the configured token is not a PocketBase auth token"}
	}
	var self map[string]any
	err := c.sendAuthenticated(ctx, http.MethodGet, "/api/collections/"+claim.CollectionID+"/records/"+claim.ID, token, &self)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
			return &AuthError{Message: "the configured token was rejected by UpSnap", Err: err}
		}
		return err
	}
	return nil
}
