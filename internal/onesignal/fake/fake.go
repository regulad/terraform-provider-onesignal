// SPDX-License-Identifier: MPL-2.0

// Package fake is an in-memory stand-in for OneSignal's configuration API, for
// tests. It follows what the live API was seen to do, quirks included, and its
// reference where nothing was seen.
package fake

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PushPlatformFlags are the template platform flags OneSignal returns.
var PushPlatformFlags = []string{"isIos", "isAndroid", "isHuawei", "isAdm", "isChrome", "isChromeWeb", "isFirefox", "isSafari", "isEdge", "isMacOSX", "isWP", "isWP_WNS", "isAlexa"}

// defaultPushPlatforms are those a push template created without flags targets.
var defaultPushPlatforms = map[string]bool{"isChromeWeb": true, "isEdge": true, "isFirefox": true}

const accessDenied = "Access denied.  Please include an 'Authorization: ...' header with a valid API key (https://documentation.onesignal.com/docs/en/keys-and-ids#api-keys)."

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

// writableAppFields are those create and update store. site_name and
// chrome_web_default_notification_icon are accepted and dropped, as live.
var writableAppFields = map[string]bool{
	"name": true, "organization_id": true, "chrome_web_origin": true,
	"safari_site_origin": true, "safari_icon_256_256": true,
	"safari_apns_p12": true, "safari_apns_p12_password": true, "fcm_v1_service_account_json": true,
	"apns_p8": true, "apns_p12": true, "apns_p12_password": true,
}

// apnsSettings are kept only alongside an APNs credential, sent with them or
// already held.
var apnsSettings = []string{"apns_env", "apns_key_id", "apns_team_id", "apns_bundle_id"}

// applyAppFields stores body's fields on app.
func applyAppFields(app, body map[string]any) {
	for k, v := range body {
		if writableAppFields[k] {
			app[k] = v
		}
	}
	if app["apns_p8"] != nil || app["apns_p12"] != nil {
		for _, k := range apnsSettings {
			if v, ok := body[k]; ok {
				app[k] = v
			}
		}
	}
	// Live, true sticks and false is dropped.
	if b, _ := body["additional_data_is_root_payload"].(bool); b {
		app["additional_data_is_root_payload"] = true
	}
	if _, ok := body["fcm_v1_service_account_json"]; ok {
		app["fcm_sender_id"] = "123456789012"
	}
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
	case parts[0] == "apps" && len(parts) == 2 && r.Method == http.MethodGet:
		// Viewing an app takes the Organization key or one of the app's keys.
		app, ok := s.apps[parts[1]]
		switch {
		case !org && s.secrets[key] != parts[1]:
			errorBody(w, http.StatusUnauthorized, accessDenied)
		case !ok:
			errorBody(w, http.StatusNotFound, "Couldn't find app with id = "+parts[1])
		default:
			write(w, http.StatusOK, app)
		}
	case parts[0] == "apps" && len(parts) <= 2:
		s.handleApps(w, r, parts, body, org)
	case parts[0] == "apps" && len(parts) >= 4 && parts[2] == "auth" && parts[3] == "tokens":
		if !org {
			errorBody(w, http.StatusForbidden, "Current organization permissions do not allow this action.")
			return
		}
		s.handleKeys(w, r, parts, body)
	case parts[0] == "apps" && len(parts) >= 3 && parts[2] == "segments":
		// Live, the Organization key is refused here.
		if s.secrets[key] != parts[1] {
			errorBody(w, http.StatusUnauthorized, accessDenied)
			return
		}
		s.handleSegments(w, r, parts, body)
	case parts[0] == "templates":
		appID := r.URL.Query().Get("app_id")
		if r.Method == http.MethodPost && len(parts) == 1 {
			appID, _ = body["app_id"].(string)
		}
		if s.secrets[key] != appID {
			errorBody(w, http.StatusUnauthorized, accessDenied)
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
		applyAppFields(app, body)
		s.apps[id] = app
		write(w, http.StatusOK, map[string]any{"id": id, "name": app["name"], "organization_id": app["organization_id"]})
	case r.Method == http.MethodPut && len(parts) == 2:
		app, ok := s.apps[parts[1]]
		if !ok {
			errorBody(w, http.StatusNotFound, "Couldn't find app with id = "+parts[1])
			return
		}
		if v, ok := body["chrome_web_origin"]; ok && v == "" {
			errorBody(w, http.StatusBadRequest, "<p>You must provide your <strong>Site URL</strong>. Include the protocol (<code class='keep-word'>http://</code> or <code class='keep-word'>https://</code>).</p>")
			return
		}
		// Live, an update merges: fields not sent are kept.
		applyAppFields(app, body)
		write(w, http.StatusOK, map[string]any{"id": app["id"], "name": app["name"]})
	default:
		errorBody(w, http.StatusNotFound, "not found")
	}
}

// storedAllowlist checks an allowlist as sent and returns it as OneSignal keeps
// it: CIDR ranges only, IPv6 in canonical form, and a /32 or /128 as a bare
// address. A nil list is returned when none was sent.
func storedAllowlist(v any) ([]any, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, v == nil
	}
	out := []any{}
	for _, e := range raw {
		s, _ := e.(string)
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, false
		}
		ones, bits := n.Mask.Size()
		if ones == bits {
			out = append(out, ip.String())
		} else {
			out = append(out, fmt.Sprintf("%s/%d", ip, ones))
		}
	}
	return out, true
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
		allowlist, ok := storedAllowlist(body["ip_allowlist"])
		if !ok {
			errorBody(w, http.StatusBadRequest, "ip_allowlist must be an array of networks in CIDR notation")
			return
		}
		id, secret := newID(), "os_v2_app_"+newID()
		k := map[string]any{"token_id": id, "name": body["name"], "ip_allowlist_mode": "disabled", "ip_allowlist": []any{}, "created_at": now, "updated_at": now}
		if v, ok := body["ip_allowlist_mode"]; ok {
			k["ip_allowlist_mode"] = v
		}
		if allowlist != nil {
			k["ip_allowlist"] = allowlist
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
		allowlist, ok := storedAllowlist(body["ip_allowlist"])
		if !ok {
			errorBody(w, http.StatusBadRequest, "ip_allowlist must be an array of networks in CIDR notation")
			return
		}
		for _, f := range []string{"name", "ip_allowlist_mode"} {
			if v, ok := body[f]; ok {
				s.keys[appID][i][f] = v
			}
		}
		if allowlist != nil {
			s.keys[appID][i]["ip_allowlist"] = allowlist
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
				filters = append(filters, decimalHoursAgo(f))
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

// decimalHoursAgo returns a filter with hours_ago written as OneSignal returns
// it on a default segment: "168.0" for 168.
func decimalHoursAgo(f any) any {
	m, ok := f.(map[string]any)
	if !ok {
		return f
	}
	raw, ok := m["hours_ago"].(string)
	if !ok {
		return f
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return f
	}
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	text := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.Contains(text, ".") {
		text += ".0"
	}
	out["hours_ago"] = text
	return out
}

// templateFields are the content fields OneSignal keeps; others are dropped.
var templateFields = map[string]bool{
	"contents": true, "headings": true, "subtitle": true, "url": true, "global_image": true,
	"email_subject": true, "email_body": true, "email_preheader": true, "email_from_name": true,
	"email_from_address": true, "email_reply_to_address": true, "email_bcc": true,
	"sms_from": true, "sms_media_urls": true, "isEmail": true, "isSMS": true,
}

func init() {
	for _, f := range PushPlatformFlags {
		templateFields[f] = true
	}
}

func templateResponse(t map[string]any) map[string]any {
	channel := "push"
	if b, _ := t["isEmail"].(bool); b {
		channel = "email"
	} else if b, _ := t["isSMS"].(bool); b {
		channel = "sms"
	}
	// Every field comes back, null where unset; contents and headings as {}.
	content := map[string]any{"contents": map[string]any{}, "headings": map[string]any{}}
	for f := range templateFields {
		if _, ok := content[f]; !ok {
			content[f] = nil
		}
	}
	for k, v := range t {
		if templateFields[k] {
			content[k] = v
		}
	}
	if channel == "email" {
		content["disable_email_click_tracking"] = false
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
		t := map[string]any{"id": id, "app_id": appID, "created_at": now, "updated_at": now, "name": body["name"]}
		flagged := false
		for k, v := range body {
			if templateFields[k] && v != nil {
				t[k] = v
			}
			if strings.HasPrefix(k, "is") && k != "isEmail" && k != "isSMS" {
				flagged = true
			}
		}
		if channel := templateResponse(t)["channel"]; channel == "push" && !flagged {
			for _, f := range PushPlatformFlags {
				t[f] = defaultPushPlatforms[f]
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
			// Live, an update merges: a field sent replaces it whole, null or ""
			// clears it, and a map sent as null fails.
			for _, m := range []string{"contents", "headings", "subtitle"} {
				if v, ok := body[m]; ok && v == nil {
					write(w, http.StatusInternalServerError, map[string]any{"status": 500, "error": "Internal Server Error"})
					return
				}
			}
			t["name"] = body["name"]
			for k, v := range body {
				switch {
				case !templateFields[k]:
				case v == nil || v == "":
					delete(t, k)
				default:
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
