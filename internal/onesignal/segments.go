// SPDX-License-Identifier: MPL-2.0

package onesignal

import (
	"context"
	"net/http"
	"net/url"
)

// Filter is one entry of a segment's flat filter list: a filter, such as
// {"field": "tag", "key": "plan", "relation": "=", "value": "pro"}, or an
// operator, {"operator": "OR"}. Operands are strings, as OneSignal stores them.
type Filter map[string]string

// Segment is a segment with its filters.
type Segment struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Filters     []Filter `json:"filters"`
	Source      string   `json:"source,omitempty"`
}

func segmentsPath(appID string) string {
	return "/apps/" + url.PathEscape(appID) + "/segments"
}

// CreateSegment creates a segment and returns its ID.
func (c *Client) CreateSegment(ctx context.Context, appKey, appID string, s Segment) (string, error) {
	var created struct {
		ID string `json:"id"`
	}
	body := Segment{ID: s.ID, Name: s.Name, Description: s.Description, Filters: s.Filters}
	err := c.do(ctx, http.MethodPost, segmentsPath(appID), nil, c.key(appKey), body, &created)
	return created.ID, err
}

// GetSegment returns a segment with its filters (include-segment-detail).
func (c *Client) GetSegment(ctx context.Context, appKey, appID, id string) (*Segment, error) {
	var view struct {
		Payload *struct {
			ID          string           `json:"id"`
			Name        string           `json:"name"`
			Description *string          `json:"description"`
			Source      string           `json:"source"`
			Filters     []map[string]any `json:"filters"`
		} `json:"payload"`
	}
	query := url.Values{"include-segment-detail": {"true"}}
	if err := c.do(ctx, http.MethodGet, segmentsPath(appID)+"/"+url.PathEscape(id), query, c.key(appKey), nil, &view); err != nil {
		return nil, err
	}
	if view.Payload == nil {
		return nil, &Error{Method: http.MethodGet, Path: segmentsPath(appID) + "/" + id, StatusCode: http.StatusNotFound, Messages: []string{"no segment detail in the response"}}
	}
	p := view.Payload
	s := &Segment{ID: p.ID, Name: p.Name, Description: p.Description, Source: p.Source}
	for _, raw := range p.Filters {
		s.Filters = append(s.Filters, stringFilter(raw))
	}
	return s, nil
}

// stringFilter turns a filter as read into strings, dropping read-only markers.
func stringFilter(raw map[string]any) Filter {
	f := Filter{}
	for k, v := range raw {
		if k == "unsupported_in_api" {
			continue
		}
		switch t := v.(type) {
		case nil:
		case string:
			f[k] = t
		default:
			f[k] = jsonScalar(t)
		}
	}
	return f
}

// UpdateSegment replaces the segment's name, description and filters in place.
// OneSignal requires the name every time, and clears a description sent as "".
func (c *Client) UpdateSegment(ctx context.Context, appKey, appID, id string, s Segment) error {
	description := ""
	if s.Description != nil {
		description = *s.Description
	}
	body := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Filters     []Filter `json:"filters"`
	}{s.Name, description, s.Filters}
	return c.do(ctx, http.MethodPatch, segmentsPath(appID)+"/"+url.PathEscape(id), nil, c.key(appKey), body, nil)
}

// DeleteSegment deletes a segment.
func (c *Client) DeleteSegment(ctx context.Context, appKey, appID, id string) error {
	return c.do(ctx, http.MethodDelete, segmentsPath(appID)+"/"+url.PathEscape(id), nil, c.key(appKey), nil, nil)
}
