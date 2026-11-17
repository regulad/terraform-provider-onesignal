// SPDX-License-Identifier: MPL-2.0

package onesignal

import (
	"context"
	"net/http"
	"net/url"
)

// App is an app as View apps returns it. Credentials are deliberately absent:
// OneSignal may echo some of them, but the provider never reads them back.
type App struct {
	ID                               string `json:"id"`
	Name                             string `json:"name"`
	OrganizationID                   string `json:"organization_id"`
	CreatedAt                        string `json:"created_at"`
	UpdatedAt                        string `json:"updated_at"`
	ChromeWebOrigin                  string `json:"chrome_web_origin"`
	ChromeWebDefaultNotificationIcon string `json:"chrome_web_default_notification_icon"`
	ChromeWebKey                     string `json:"chrome_web_key"`
	SiteName                         string `json:"site_name"`
	SafariSiteOrigin                 string `json:"safari_site_origin"`
	SafariIcon256                    string `json:"safari_icon_256_256"`
	SafariPushID                     string `json:"safari_push_id"`
	FCMSenderID                      string `json:"fcm_sender_id"`
	APNSEnv                          string `json:"apns_env"`
	APNSKeyID                        string `json:"apns_key_id"`
	APNSTeamID                       string `json:"apns_team_id"`
	APNSBundleID                     string `json:"apns_bundle_id"`
	AdditionalDataIsRootPayload      *bool  `json:"additional_data_is_root_payload"`
}

// AppFields are the writable fields of an app, by their API names. Only the
// fields present are sent, so an update leaves every other field as it is.
type AppFields map[string]any

// ListApps returns every app of the Organization key's organization.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	err := c.do(ctx, http.MethodGet, "/apps", nil, c.OrganizationKey, nil, &apps)
	return apps, err
}

// GetApp returns one app, found through ListApps, which takes the Organization
// key, where View an app is documented with an App API key only.
func (c *Client) GetApp(ctx context.Context, id string) (*App, error) {
	apps, err := c.ListApps(ctx)
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].ID == id {
			return &apps[i], nil
		}
	}
	return nil, &Error{Method: http.MethodGet, Path: "/apps", StatusCode: http.StatusNotFound, Messages: []string{"no app " + id + " in the organization"}}
}

// CreateApp creates an app and returns its ID.
func (c *Client) CreateApp(ctx context.Context, fields AppFields) (string, error) {
	var created struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/apps", nil, c.OrganizationKey, fields, &created)
	return created.ID, err
}

// UpdateApp sends fields to the app; OneSignal's response is a subset of the app,
// so callers read it again.
func (c *Client) UpdateApp(ctx context.Context, id string, fields AppFields) error {
	return c.do(ctx, http.MethodPut, "/apps/"+url.PathEscape(id), nil, c.OrganizationKey, fields, nil)
}
