// SPDX-License-Identifier: MPL-2.0

// Package fake is an in-memory stand-in for OneSignal's configuration API, as
// its reference documents it, quirks included, for tests.
package fake

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// PushPlatformFlags are the template platform flags OneSignal returns.
var PushPlatformFlags = []string{"isIos", "isAndroid", "isHuawei", "isAnyWeb", "isChromeWeb", "isFirefox", "isSafari", "isWP_WNS", "isAdm"}

// Server is a fake OneSignal API.
type Server struct {
	*httptest.Server
	OrganizationKey string
	OrganizationID  string

	mu          sync.Mutex
	apps        map[string]map[string]any
	keys        map[string][]map[string]any // by app ID
	secrets     map[string]string           // key value -> app ID
	tokenSecret map[string]string           // token ID -> its current value
	segments    map[string]map[string]map[string]any
	templates   map[string]map[string]any // by template ID, with "app_id"
	// Requests counts calls by "METHOD path", for tests.
	Requests map[string]int
}

// New starts a fake server with one organization key.
func New() *Server {
	s := &Server{
		OrganizationKey: "os_v2_org_fake",
		OrganizationID:  newID(),
		apps:            map[string]map[string]any{},
		keys:            map[string][]map[string]any{},
		secrets:         map[string]string{},
		tokenSecret:     map[string]string{},
		segments:        map[string]map[string]map[string]any{},
		templates:       map[string]map[string]any{},
		Requests:        map[string]int{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// AddApp inserts an app as if made in the dashboard and returns its ID.
func (s *Server) AddApp(name string, fields map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	app := map[string]any{"id": id, "name": name, "organization_id": s.OrganizationID, "players": 3, "messageable_players": 2}
	for k, v := range fields {
		app[k] = v
	}
	s.apps[id] = app
	return id
}

// App returns a copy of an app's stored fields, credentials included.
func (s *Server) App(id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for k, v := range s.apps[id] {
		out[k] = v
	}
	return out
}

// KeyWorks reports whether value is a live key of appID.
func (s *Server) KeyWorks(appID, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secrets[value] == appID
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func errorBody(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]any{"errors": []string{msg}})
}

// writableAppFields are those create and update accept.
var writableAppFields = map[string]bool{
	"name": true, "organization_id": true, "chrome_web_origin": true, "site_name": true,
	"safari_site_origin": true, "chrome_web_default_notification_icon": true, "safari_icon_256_256": true,
	"safari_apns_p12": true, "safari_apns_p12_password": true, "fcm_v1_service_account_json": true,
	"apns_p8": true, "apns_env": true, "apns_key_id": true, "apns_team_id": true, "apns_bundle_id": true,
	"apns_p12": true, "apns_p12_password": true, "additional_data_is_root_payload": true,
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests[r.Method+" "+r.URL.Path]++
	if r.Header.Get("Content-Type") != "application/json" {
		errorBody(w, http.StatusBadRequest, "app_id not found. You may be missing a Content-Type: application/json header.")
		return
	}
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Key ")
	org := key == s.OrganizationKey
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case parts[0] == "apps" && len(parts) <= 2:
		s.handleApps(w, r, parts, body, org)
	case parts[0] == "apps" && len(parts) >= 4 && parts[2] == "auth" && parts[3] == "tokens":
		if !org {
			errorBody(w, http.StatusForbidden, "Current organization permissions do not allow this action.")
			return
		}
		s.handleKeys(w, r, parts, body)
	case parts[0] == "apps" && len(parts) >= 3 && parts[2] == "segments":
		if !org && s.secrets[key] != parts[1] {
			errorBody(w, http.StatusUnauthorized, "Access denied.")
			return
		}
		s.handleSegments(w, r, parts, body)
	case parts[0] == "templates":
		appID := r.URL.Query().Get("app_id")
		if r.Method == http.MethodPost && len(parts) == 1 {
			appID, _ = body["app_id"].(string)
		}
		if !org && s.secrets[key] != appID {
			errorBody(w, http.StatusUnauthorized, "Access denied.")
			return
		}
		s.handleTemplates(w, r, parts, body, appID)
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request, parts []string, body map[string]any, org bool) {
	if !org {
		errorBody(w, http.StatusForbidden, "You do not have permission to list apps.")
		return
	}
	switch {
	case r.Method == http.MethodGet && len(parts) == 1:
		list := []map[string]any{}
		for _, app := range s.apps {
			list = append(list, app)
		}
		write(w, http.StatusOK, list)
	case r.Method == http.MethodPost && len(parts) == 1:
		if body["name"] == nil || body["organization_id"] != s.OrganizationID {
			errorBody(w, http.StatusBadRequest, "name and organization_id are required")
			return
		}
		id := newID()
		app := map[string]any{"id": id, "players": 0, "messageable_players": 0}
		for k, v := range body {
			if writableAppFields[k] {
				app[k] = v
			}
		}
		s.apps[id] = app
		write(w, http.StatusOK, map[string]any{"id": id, "name": app["name"], "organization_id": app["organization_id"]})
	case r.Method == http.MethodPut && len(parts) == 2:
		app, ok := s.apps[parts[1]]
		if !ok {
			errorBody(w, http.StatusNotFound, "Couldn't find app with id = "+parts[1])
			return
		}
		// The fake merges: fields not sent are kept.
		for k, v := range body {
			if writableAppFields[k] {
				app[k] = v
			}
		}
		if _, ok := body["fcm_v1_service_account_json"]; ok {
			app["fcm_sender_id"] = "123456789012"
		}
		write(w, http.StatusOK, map[string]any{"id": app["id"], "name": app["name"]})
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request, parts []string, body map[string]any) {
	appID := parts[1]
	if _, ok := s.apps[appID]; !ok {
		errorBody(w, http.StatusNotFound, "App not found")
		return
	}
	find := func(id string) int {
		for i, k := range s.keys[appID] {
			if k["token_id"] == id {
				return i
			}
		}
		return -1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case r.Method == http.MethodGet && len(parts) == 4:
		write(w, http.StatusOK, map[string]any{"tokens": s.keys[appID]})
	case r.Method == http.MethodPost && len(parts) == 4:
		if body["name"] == nil {
			write(w, http.StatusBadRequest, map[string]any{})
			return
		}
		id, secret := newID(), "os_v2_app_"+newID()
		k := map[string]any{"token_id": id, "name": body["name"], "ip_allowlist_mode": "disabled", "ip_allowlist": []any{}, "created_at": now, "updated_at": now}
		if v, ok := body["ip_allowlist_mode"]; ok {
			k["ip_allowlist_mode"] = v
		}
		if v, ok := body["ip_allowlist"]; ok && v != nil {
			k["ip_allowlist"] = v
		}
		s.keys[appID] = append(s.keys[appID], k)
		s.secrets[secret] = appID
		s.tokenSecret[id] = secret
		write(w, http.StatusOK, map[string]any{"token_id": id, "formatted_token": secret})
	case r.Method == http.MethodPatch && len(parts) == 5:
		i := find(parts[4])
		if i < 0 {
			errorBody(w, http.StatusNotFound, "Token not found")
			return
		}
		for _, f := range []string{"name", "ip_allowlist_mode", "ip_allowlist"} {
			if v, ok := body[f]; ok {
				s.keys[appID][i][f] = v
			}
		}
		s.keys[appID][i]["updated_at"] = now
		write(w, http.StatusOK, map[string]any{})
	case r.Method == http.MethodPost && len(parts) == 6 && parts[5] == "rotate":
		if find(parts[4]) < 0 {
			errorBody(w, http.StatusNotFound, "Token not found")
			return
		}
		// The old value stops working at once.
		delete(s.secrets, s.tokenSecret[parts[4]])
		secret := "os_v2_app_" + newID()
		s.secrets[secret] = appID
		s.tokenSecret[parts[4]] = secret
		write(w, http.StatusOK, map[string]any{"formatted_token": secret})
	case r.Method == http.MethodDelete && len(parts) == 5:
		i := find(parts[4])
		if i < 0 {
			errorBody(w, http.StatusNotFound, "Token not found")
			return
		}
		s.keys[appID] = append(s.keys[appID][:i], s.keys[appID][i+1:]...)
		delete(s.secrets, s.tokenSecret[parts[4]])
		delete(s.tokenSecret, parts[4])
		write(w, http.StatusOK, map[string]any{})
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request, parts []string, body map[string]any) {
	appID := parts[1]
	if s.segments[appID] == nil {
		s.segments[appID] = map[string]map[string]any{}
	}
	segs := s.segments[appID]
	switch {
	case r.Method == http.MethodPost && len(parts) == 3:
		filters, _ := body["filters"].([]any)
		if body["name"] == nil || len(filters) == 0 {
			errorBody(w, http.StatusBadRequest, "name and filters are required")
			return
		}
		id, _ := body["id"].(string)
		if id == "" {
			id = newID()
		}
		seg := map[string]any{"id": id, "name": body["name"], "description": body["description"], "filters": filters, "source": "custom"}
		segs[id] = seg
		write(w, http.StatusCreated, map[string]any{"success": true, "id": id})
	case r.Method == http.MethodGet && len(parts) == 4:
		seg, ok := segs[parts[3]]
		if !ok {
			errorBody(w, http.StatusNotFound, "segment not found")
			return
		}
		out := map[string]any{"subscriber_count": 7}
		if r.URL.Query().Get("include-segment-detail") == "true" {
			// The API spells out the AND that joins adjacent filters.
			var filters []any
			stored, _ := seg["filters"].([]any)
			for i, f := range stored {
				if i > 0 {
					if m, ok := f.(map[string]any); ok && m["operator"] == nil {
						if prev, ok := stored[i-1].(map[string]any); ok && prev["operator"] == nil {
							filters = append(filters, map[string]any{"operator": "AND"})
						}
					}
				}
				filters = append(filters, f)
			}
			out["payload"] = map[string]any{"id": seg["id"], "name": seg["name"], "description": seg["description"], "source": seg["source"], "filters": filters, "created_at": 1700000000}
		}
		write(w, http.StatusOK, out)
	case r.Method == http.MethodPatch && len(parts) == 4:
		seg, ok := segs[parts[3]]
		if !ok {
			write(w, http.StatusNotFound, map[string]any{"success": false, "errors": []string{"segment not found"}})
			return
		}
		if body["name"] == nil {
			errorBody(w, http.StatusBadRequest, "name is required")
			return
		}
		seg["name"] = body["name"]
		if d, ok := body["description"]; ok {
			if d == "" {
				seg["description"] = nil
			} else {
				seg["description"] = d
			}
		}
		if f, ok := body["filters"].([]any); ok {
			if len(f) == 0 {
				errorBody(w, http.StatusBadRequest, "filters cannot be empty")
				return
			}
			seg["filters"] = f
		}
		write(w, http.StatusOK, map[string]any{"success": true, "id": seg["id"]})
	case r.Method == http.MethodDelete && len(parts) == 4:
		if _, ok := segs[parts[3]]; !ok {
			write(w, http.StatusNotFound, map[string]any{"success": false})
			return
		}
		delete(segs, parts[3])
		write(w, http.StatusOK, map[string]any{"success": true})
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}

func templateResponse(t map[string]any) map[string]any {
	content := map[string]any{}
	channel := "push"
	if b, _ := t["isEmail"].(bool); b {
		channel = "email"
	} else if b, _ := t["isSMS"].(bool); b {
		channel = "SMS"
	}
	for k, v := range t {
		switch k {
		case "id", "name", "app_id", "created_at", "updated_at":
		default:
			content[k] = v
		}
	}
	if channel == "push" {
		// Every flag comes back: true where targeted, null otherwise; all true
		// when none was set.
		set := false
		for _, f := range PushPlatformFlags {
			if b, _ := t[f].(bool); b {
				set = true
			}
		}
		for _, f := range PushPlatformFlags {
			if !set {
				content[f] = true
			} else if b, _ := t[f].(bool); b {
				content[f] = true
			} else {
				content[f] = nil
			}
		}
	}
	return map[string]any{"id": t["id"], "name": t["name"], "channel": channel, "created_at": t["created_at"], "updated_at": t["updated_at"], "content": content}
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request, parts []string, body map[string]any, appID string) {
	now := time.Now().UTC().Format(time.RFC3339)
	switch {
	case r.Method == http.MethodPost && len(parts) == 1:
		if body["name"] == nil {
			errorBody(w, http.StatusBadRequest, "name is required")
			return
		}
		if b, _ := body["isEmail"].(bool); b {
			if body["email_body"] == nil {
				write(w, http.StatusBadRequest, map[string]any{"success": false, "errors": []string{"'message' Email body can not be blank"}})
				return
			}
		} else if _, ok := body["contents"].(map[string]any); !ok {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte("<html><head><title>The change you wanted was rejected (422)</title></head></html>"))
			return
		}
		id := newID()
		t := map[string]any{"id": id, "app_id": appID, "created_at": now, "updated_at": now}
		for k, v := range body {
			if k != "app_id" {
				t[k] = v
			}
		}
		s.templates[id] = t
		write(w, http.StatusOK, templateResponse(t))
	case len(parts) == 2:
		t, ok := s.templates[parts[1]]
		if !ok || t["app_id"] != appID {
			errorBody(w, http.StatusNotFound, "template not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			write(w, http.StatusOK, templateResponse(t))
		case http.MethodPatch:
			if body["name"] == nil {
				errorBody(w, http.StatusBadRequest, "name is required")
				return
			}
			// The fake merges, and clears a field sent as null.
			for k, v := range body {
				if v == nil {
					delete(t, k)
				} else {
					t[k] = v
				}
			}
			t["updated_at"] = now
			write(w, http.StatusOK, templateResponse(t))
		case http.MethodDelete:
			delete(s.templates, parts[1])
			write(w, http.StatusOK, map[string]any{"success": true})
		default:
			errorBody(w, http.StatusNotFound, "not found")
		}
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}
