// SPDX-License-Identifier: MPL-2.0

// Package onesignal is a client for OneSignal's configuration API: apps, app API
// keys, segments and templates.
package onesignal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is OneSignal's API.
const DefaultBaseURL = "https://api.onesignal.com"

// Client calls OneSignal's API. Apps and their API keys take OrganizationKey.
// Segments and templates take an App API key per call: OneSignal refuses the
// Organization key there.
type Client struct {
	BaseURL         string
	OrganizationKey string
	HTTPClient      *http.Client
	UserAgent       string
	// MaxRetries bounds the retries of a 429 or 503.
	MaxRetries int
	// MinTemplateViewInterval spaces template views per app, which OneSignal
	// limits to one request per second per app.
	MinTemplateViewInterval time.Duration

	mu            sync.Mutex
	templateViews map[string]time.Time
}

// New returns a client for baseURL, or DefaultBaseURL when it is empty.
func New(baseURL, organizationKey, userAgent string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:                 strings.TrimRight(baseURL, "/"),
		OrganizationKey:         organizationKey,
		HTTPClient:              &http.Client{Timeout: 110 * time.Second},
		UserAgent:               userAgent,
		MaxRetries:              5,
		MinTemplateViewInterval: time.Second,
		templateViews:           map[string]time.Time{},
	}
}

// Error is a response OneSignal refused, with its messages.
type Error struct {
	Method     string
	Path       string
	StatusCode int
	Messages   []string
}

func (e *Error) Error() string {
	msg := strings.Join(e.Messages, "; ")
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("OneSignal %s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, msg)
}

// IsNotFound reports whether err is OneSignal's 404.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// ErrNotFound is returned for an object a listing does not contain.
var ErrNotFound = &Error{StatusCode: http.StatusNotFound, Messages: []string{"not found"}}

// do sends a request with key and decodes a 2xx JSON body into out, if out is
// not nil. It retries a 429 or 503, honouring Retry-After.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, key string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding the request to %s %s: %w", method, path, err)
		}
	}
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Key "+key)
		// OneSignal requires it on every request, GET and DELETE included.
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("OneSignal %s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading OneSignal's response to %s %s: %w", method, path, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if attempt < c.MaxRetries {
				if err := sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return &Error{Method: method, Path: path, StatusCode: resp.StatusCode, Messages: errorMessages(data)}
		}
		if out != nil && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("decoding OneSignal's response to %s %s: %w", method, path, err)
			}
		}
		return nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	backoff := time.Duration(1<<attempt) * time.Second
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && time.Duration(seconds)*time.Second > backoff {
		return time.Duration(seconds) * time.Second
	}
	return backoff
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var htmlTitle = regexp.MustCompile(`(?is)<title>(.*?)</title>`)

// errorMessages extracts OneSignal's messages from any of its error bodies:
// {"errors": [string]}, {"errors": [{code, title}]}, {"errors": string}, {}
// and, on some 422s, an HTML page.
func errorMessages(data []byte) []string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}
	var envelope struct {
		Errors json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(trimmed, &envelope) == nil {
		if len(envelope.Errors) == 0 {
			if string(trimmed) == "{}" {
				return nil
			}
			return []string{string(trimmed)}
		}
		var list []json.RawMessage
		if json.Unmarshal(envelope.Errors, &list) != nil {
			list = []json.RawMessage{envelope.Errors}
		}
		var out []string
		for _, item := range list {
			var s string
			if json.Unmarshal(item, &s) == nil {
				out = append(out, plainText(s))
				continue
			}
			var structured struct {
				Code  string `json:"code"`
				Title string `json:"title"`
			}
			if json.Unmarshal(item, &structured) == nil && (structured.Title != "" || structured.Code != "") {
				out = append(out, strings.TrimSpace(structured.Code+" "+structured.Title))
				continue
			}
			out = append(out, string(item))
		}
		return out
	}
	if m := htmlTitle.FindSubmatch(trimmed); m != nil {
		return []string{strings.TrimSpace(string(m[1]))}
	}
	text := string(trimmed)
	if len(text) > 300 {
		text = text[:300]
	}
	return []string{text}
}

var (
	htmlTag    = regexp.MustCompile(`<[^>]*>`)
	whitespace = regexp.MustCompile(`\s+`)
)

// plainText drops the HTML tags some of OneSignal's messages carry.
func plainText(s string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(htmlTag.ReplaceAllString(s, ""), " "))
}

// waitTemplateView spaces template views of one app by MinTemplateViewInterval.
func (c *Client) waitTemplateView(ctx context.Context, appID string) error {
	for {
		c.mu.Lock()
		next := c.templateViews[appID].Add(c.MinTemplateViewInterval)
		now := time.Now()
		if !now.Before(next) {
			c.templateViews[appID] = now
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		if err := sleep(ctx, next.Sub(now)); err != nil {
			return err
		}
	}
}
