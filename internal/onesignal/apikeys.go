// SPDX-License-Identifier: MPL-2.0

package onesignal

import (
	"context"
	"net/http"
	"net/url"
)

// IP allowlist modes.
const (
	AllowlistDisabled = "disabled"
	AllowlistExplicit = "explicit"
)

// APIKey is an App API key. Its value, FormattedToken, comes back only from
// create and rotate; OneSignal does not store it.
type APIKey struct {
	TokenID         string   `json:"token_id"`
	Name            string   `json:"name"`
	IPAllowlistMode string   `json:"ip_allowlist_mode"`
	IPAllowlist     []string `json:"ip_allowlist"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	FormattedToken  string   `json:"formatted_token,omitempty"`
}

// APIKeyInput is what create and update send.
type APIKeyInput struct {
	Name            string   `json:"name"`
	IPAllowlistMode string   `json:"ip_allowlist_mode"`
	IPAllowlist     []string `json:"ip_allowlist"`
}

// NewAPIKeyInput derives the allowlist mode from the list: explicit when it holds
// any CIDR, disabled when it is empty.
func NewAPIKeyInput(name string, allowlist []string) APIKeyInput {
	mode := AllowlistDisabled
	if len(allowlist) > 0 {
		mode = AllowlistExplicit
	}
	if allowlist == nil {
		allowlist = []string{}
	}
	return APIKeyInput{Name: name, IPAllowlistMode: mode, IPAllowlist: allowlist}
}

func tokensPath(appID string) string {
	return "/apps/" + url.PathEscape(appID) + "/auth/tokens"
}

// CreateAPIKey creates a key; the result carries its TokenID and its value.
func (c *Client) CreateAPIKey(ctx context.Context, appID string, in APIKeyInput) (*APIKey, error) {
	var created APIKey
	err := c.do(ctx, http.MethodPost, tokensPath(appID), nil, c.OrganizationKey, in, &created)
	return &created, err
}

// ListAPIKeys returns the app's keys, without their values.
func (c *Client) ListAPIKeys(ctx context.Context, appID string) ([]APIKey, error) {
	var listing struct {
		Tokens []APIKey `json:"tokens"`
	}
	err := c.do(ctx, http.MethodGet, tokensPath(appID), nil, c.OrganizationKey, nil, &listing)
	return listing.Tokens, err
}

// GetAPIKey finds one key in the listing, as OneSignal has no endpoint for one.
func (c *Client) GetAPIKey(ctx context.Context, appID, tokenID string) (*APIKey, error) {
	keys, err := c.ListAPIKeys(ctx, appID)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].TokenID == tokenID {
			return &keys[i], nil
		}
	}
	return nil, &Error{Method: http.MethodGet, Path: tokensPath(appID), StatusCode: http.StatusNotFound, Messages: []string{"no API key " + tokenID}}
}

// UpdateAPIKey sends the key's name and allowlist; OneSignal answers {}.
func (c *Client) UpdateAPIKey(ctx context.Context, appID, tokenID string, in APIKeyInput) error {
	return c.do(ctx, http.MethodPatch, tokensPath(appID)+"/"+url.PathEscape(tokenID), nil, c.OrganizationKey, in, nil)
}

// RotateAPIKey gives the key a new value, keeping its ID, name and allowlist.
func (c *Client) RotateAPIKey(ctx context.Context, appID, tokenID string) (string, error) {
	var rotated struct {
		FormattedToken string `json:"formatted_token"`
	}
	err := c.do(ctx, http.MethodPost, tokensPath(appID)+"/"+url.PathEscape(tokenID)+"/rotate", nil, c.OrganizationKey, nil, &rotated)
	return rotated.FormattedToken, err
}

// DeleteAPIKey revokes the key.
func (c *Client) DeleteAPIKey(ctx context.Context, appID, tokenID string) error {
	return c.do(ctx, http.MethodDelete, tokensPath(appID)+"/"+url.PathEscape(tokenID), nil, c.OrganizationKey, nil, nil)
}
