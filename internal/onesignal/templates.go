// SPDX-License-Identifier: MPL-2.0

package onesignal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Template channels, as the provider names them.
const (
	ChannelPush  = "push"
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

// TemplateFields are a template's content fields by their API names, sent as
// given. The channel flags isEmail and isSMS are set from the channel.
type TemplateFields map[string]any

// Template is a template as OneSignal returns it.
type Template struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Channel   string         `json:"channel"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Content   map[string]any `json:"content"`
}

// NormalizedChannel returns the channel in lower case; OneSignal spells SMS in
// capitals. When the response carries no channel it is read from the content.
func (t *Template) NormalizedChannel() string {
	if t.Channel != "" {
		return strings.ToLower(t.Channel)
	}
	if b, _ := t.Content["isEmail"].(bool); b {
		return ChannelEmail
	}
	if b, _ := t.Content["isSMS"].(bool); b {
		return ChannelSMS
	}
	return ChannelPush
}

func templateBody(appID, name, channel string, fields TemplateFields) map[string]any {
	body := map[string]any{"name": name}
	if appID != "" {
		body["app_id"] = appID
	}
	for k, v := range fields {
		body[k] = v
	}
	switch channel {
	case ChannelEmail:
		body["isEmail"] = true
	case ChannelSMS:
		body["isSMS"] = true
	}
	return body
}

func templatePath(id string) string {
	return "/templates/" + url.PathEscape(id)
}

// CreateTemplate creates a template; the app ID travels in the body.
func (c *Client) CreateTemplate(ctx context.Context, appKey, appID, name, channel string, fields TemplateFields) (*Template, error) {
	var t Template
	err := c.do(ctx, http.MethodPost, "/templates", nil, appKey, templateBody(appID, name, channel, fields), &t)
	return &t, err
}

// GetTemplate returns a template, waiting out OneSignal's view limit.
func (c *Client) GetTemplate(ctx context.Context, appKey, appID, id string) (*Template, error) {
	if err := c.waitTemplateView(ctx, appID); err != nil {
		return nil, err
	}
	var t Template
	err := c.do(ctx, http.MethodGet, templatePath(id), url.Values{"app_id": {appID}}, appKey, nil, &t)
	return &t, err
}

// UpdateTemplate changes a template in place; OneSignal requires the name.
func (c *Client) UpdateTemplate(ctx context.Context, appKey, appID, id, name, channel string, fields TemplateFields) error {
	return c.do(ctx, http.MethodPatch, templatePath(id), url.Values{"app_id": {appID}}, appKey, templateBody("", name, channel, fields), nil)
}

// DeleteTemplate deletes a template.
func (c *Client) DeleteTemplate(ctx context.Context, appKey, appID, id string) error {
	return c.do(ctx, http.MethodDelete, templatePath(id), url.Values{"app_id": {appID}}, appKey, nil, nil)
}

// jsonScalar renders a non-string JSON scalar as the string OneSignal would
// accept for it.
func jsonScalar(v any) string {
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}
