// SPDX-License-Identifier: MPL-2.0

package onesignal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestErrorMessages(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"strings":    {`{"errors": ["Segment name is already taken."]}`, []string{"Segment name is already taken."}},
		"success":    {`{"success": false, "errors": ["template not found"]}`, []string{"template not found"}},
		"structured": {`{"errors": [{"code": "invalid", "title": "Bad offset"}]}`, []string{"invalid Bad offset"}},
		"single":     {`{"errors": "API rate limit exceeded"}`, []string{"API rate limit exceeded"}},
		"empty":      {`{}`, nil},
		"html":       {`<html><head><title>The change you wanted was rejected (422)</title></head></html>`, []string{"The change you wanted was rejected (422)"}},
		"nothing":    {``, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := errorMessages([]byte(tc.body)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("errorMessages(%q) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestRetriesRateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Key org" {
			t.Errorf("headers: %v", r.Header)
		}
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"errors":["API rate limit exceeded"]}`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"a","name":"x"}]`))
	}))
	defer srv.Close()
	c := New(srv.URL, "org", "test")
	apps, err := c.ListApps(context.Background())
	if err != nil || len(apps) != 1 || calls.Load() != 2 {
		t.Fatalf("ListApps: %v, %v apps, %d calls", err, len(apps), calls.Load())
	}
}

func TestErrorsAreNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Couldn't find app"]}`))
	}))
	defer srv.Close()
	err := New(srv.URL, "org", "test").UpdateApp(context.Background(), "x", AppFields{"name": "y"})
	if !IsNotFound(err) || calls.Load() != 1 {
		t.Fatalf("got %v after %d calls", err, calls.Load())
	}
}

func TestAppKeyFallsBackToOrganizationKey(t *testing.T) {
	c := New("", "org", "test")
	if c.key("") != "org" || c.key("app") != "app" {
		t.Fatal("key fallback")
	}
}

func TestTemplateViewsAreSpaced(t *testing.T) {
	c := New("", "org", "test")
	c.MinTemplateViewInterval = 50 * time.Millisecond
	start := time.Now()
	for range 3 {
		if err := c.waitTemplateView(context.Background(), "app"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("three views took %v, want at least 100ms", elapsed)
	}
}

func TestNewAPIKeyInputDerivesMode(t *testing.T) {
	if in := NewAPIKeyInput("k", nil); in.IPAllowlistMode != AllowlistDisabled || in.IPAllowlist == nil {
		t.Fatalf("empty: %+v", in)
	}
	if in := NewAPIKeyInput("k", []string{"2001:db8::/32"}); in.IPAllowlistMode != AllowlistExplicit {
		t.Fatalf("set: %+v", in)
	}
}
