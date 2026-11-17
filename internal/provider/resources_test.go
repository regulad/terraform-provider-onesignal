// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal/fake"
)

// Steps that expect a configuration to be refused come first in each test: the
// test's closing destroy runs with the last step's configuration, which must be
// valid.

func TestAccApp(t *testing.T) {
	srv := fake.New()
	defer srv.Close()
	cfg := func(name, p8 string, version int) string {
		return providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_app" "test" {
  name               = %q
  organization_id    = %q
  chrome_web_origin  = "https://example.com"
  apns_key_id        = "ABCDE12345"
  apns_p8_wo         = %q
  apns_p8_wo_version = %d
}
`, name, srv.OrganizationID, p8, version)
	}
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("first", "cDggb25l", 1),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_app.test", "name", "first"),
					resource.TestCheckResourceAttr("onesignal_app.test", "web_configured", "true"),
					resource.TestCheckNoResourceAttr("onesignal_app.test", "apns_p8_wo"),
					func(s *terraform.State) error {
						var err error
						if id, err = stateAttr(s, "onesignal_app.test", "id"); err != nil {
							return err
						}
						if got := srv.App(id)["apns_p8"]; got != "cDggb25l" {
							return fmt.Errorf("apns_p8 on create: %v", got)
						}
						return nil
					},
				),
			},
			{
				// A rename alone leaves the credential alone.
				Config: cfg("second", "ignored-until-version-moves", 1),
				Check: func(*terraform.State) error {
					app := srv.App(id)
					if app["name"] != "second" || app["apns_p8"] != "cDggb25l" {
						return fmt.Errorf("after rename: name %v, apns_p8 %v", app["name"], app["apns_p8"])
					}
					return nil
				},
			},
			{
				// Moving the version resends it, and nothing else is sent.
				Config: cfg("second", "cDggdHdv", 2),
				Check: func(*terraform.State) error {
					app := srv.App(id)
					if app["apns_p8"] != "cDggdHdv" || app["chrome_web_origin"] != "https://example.com" {
						return fmt.Errorf("after version bump: %v", app)
					}
					return nil
				},
			},
			{
				ResourceName:            "onesignal_app.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"apns_p8_wo_version"},
			},
			{
				// Dropping the app from configuration plans a destroy, which fails.
				Config:      providerConfig(srv),
				ExpectError: regexp.MustCompile(`cannot delete apps`),
			},
			{
				Config: providerConfig(srv) + `
removed {
  from = onesignal_app.test
  lifecycle {
    destroy = false
  }
}
`,
			},
		},
	})
}

func TestAccAppAPIKey(t *testing.T) {
	srv := fake.New()
	defer srv.Close()
	appID := srv.AddApp("app", nil)
	cfg := func(name, allowlist, rotation string) string {
		return providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_app_api_key" "test" {
  app_id              = %q
  name                = %q
  ip_allowlist        = %s
  rotate_when_changed = { at = %q }
}
`, appID, name, allowlist, rotation)
	}
	var first string
	value := func(s *terraform.State) (string, error) { return stateAttr(s, "onesignal_app_api_key.test", "value") }
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		CheckDestroy: func(*terraform.State) error {
			if srv.KeyWorks(appID, first) {
				return fmt.Errorf("the key still works after destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config:      cfg("bad", `["not-a-cidr"]`, "1"),
				ExpectError: regexp.MustCompile(`Invalid CIDR range`),
			},
			{
				Config: cfg("one", `["192.0.2.10/32", "2001:db8::10/128"]`, "1"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_app_api_key.test", "ip_allowlist.#", "2"),
					func(s *terraform.State) error {
						v, err := value(s)
						if err != nil || !srv.KeyWorks(appID, v) {
							return fmt.Errorf("the created key's value does not work: %v", err)
						}
						first = v
						return nil
					},
				),
			},
			{
				// A rename and a new allowlist keep the key and its value.
				Config: cfg("two", `[]`, "1"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_app_api_key.test", "name", "two"),
					resource.TestCheckResourceAttr("onesignal_app_api_key.test", "ip_allowlist.#", "0"),
					func(s *terraform.State) error {
						if v, _ := value(s); v != first {
							return fmt.Errorf("the value changed without a rotation")
						}
						return nil
					},
				),
			},
			{
				// A rotation gives a new value and revokes the old one.
				Config: cfg("two", `[]`, "2"),
				Check: func(s *terraform.State) error {
					v, _ := value(s)
					if v == first || !srv.KeyWorks(appID, v) || srv.KeyWorks(appID, first) {
						return fmt.Errorf("rotation: new %v works %v, old works %v", v != first, srv.KeyWorks(appID, v), srv.KeyWorks(appID, first))
					}
					first = v
					return nil
				},
			},
			{
				ResourceName: "onesignal_app_api_key.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					id, err := stateAttr(s, "onesignal_app_api_key.test", "id")
					return appID + "/" + id, err
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value", "rotate_when_changed"},
			},
		},
	})
}

func TestAccSegment(t *testing.T) {
	srv := fake.New()
	defer srv.Close()
	appID := srv.AddApp("app", nil)
	cfg := func(description, country string) string {
		return providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_segment" "test" {
  app_id      = %q
  name        = "Engaged"
  description = %s
  filters = [
    { field = "session_count", relation = ">", value = "5" },
    { field = "country", relation = "=", value = %q },
    { operator = "OR" },
    { field = "tag", key = "plan", relation = "=", value = "pro" },
  ]
}
`, appID, description, country)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_segment" "test" {
  app_id  = %q
  name    = "Bad"
  filters = [{ field = "country", relation = "=", value = "US" }, { operator = "AND" }, { field = "language", relation = "=", value = "en" }]
}
`, appID),
				ExpectError: regexp.MustCompile(`Only OR is written`),
			},
			{
				// The API spells out the AND between the first two filters; the
				// plan stays empty, so it was dropped on read.
				Config: cfg(`"Frequent users"`, "US"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_segment.test", "filters.#", "4"),
					resource.TestCheckResourceAttr("onesignal_segment.test", "filters.2.operator", "OR"),
					resource.TestCheckResourceAttr("onesignal_segment.test", "source", "custom"),
				),
			},
			{
				Config: cfg(`null`, "CA"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr("onesignal_segment.test", "description"),
					resource.TestCheckResourceAttr("onesignal_segment.test", "filters.1.value", "CA"),
				),
			},
			{
				ResourceName: "onesignal_segment.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					id, err := stateAttr(s, "onesignal_segment.test", "id")
					return appID + "/" + id, err
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccTemplate(t *testing.T) {
	srv := fake.New()
	defer srv.Close()
	appID := srv.AddApp("app", nil)
	cfg := func(headings string) string {
		return providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_template" "push" {
  app_id         = %q
  name           = "Welcome"
  channel        = "push"
  contents       = { en = "Hello", es = "Hola" }
  headings       = %s
  url            = "https://example.com/welcome"
  push_platforms = ["ios", "android"]
}

resource "onesignal_template" "email" {
  app_id        = %q
  name          = "Receipt"
  channel       = "email"
  email_subject = "Your receipt"
  email_body    = "<p>Thanks</p>"
}

resource "onesignal_template" "sms" {
  app_id   = %q
  name     = "Code"
  channel  = "sms"
  contents = { en = "Your code is 123" }
}
`, appID, headings, appID, appID)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv) + fmt.Sprintf(`
resource "onesignal_template" "bad" {
  app_id   = %q
  name     = "No English"
  channel  = "push"
  contents = { es = "Hola" }
}
`, appID),
				ExpectError: regexp.MustCompile(`Missing English contents`),
			},
			{
				Config: cfg(`{ en = "Welcome" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_template.push", "push_platforms.#", "2"),
					resource.TestCheckResourceAttr("onesignal_template.push", "headings.en", "Welcome"),
					resource.TestCheckResourceAttr("onesignal_template.email", "channel", "email"),
					resource.TestCheckResourceAttr("onesignal_template.sms", "channel", "sms"),
				),
			},
			{
				// Removing headings clears them.
				Config: cfg(`null`),
				Check:  resource.TestCheckNoResourceAttr("onesignal_template.push", "headings.%"),
			},
			{
				ResourceName: "onesignal_template.push",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					id, err := stateAttr(s, "onesignal_template.push", "id")
					return appID + "/" + id, err
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"push_platforms"},
			},
		},
	})
}

func TestAccAppDataSources(t *testing.T) {
	srv := fake.New()
	defer srv.Close()
	appID := srv.AddApp("Doppelganger", map[string]any{"chrome_web_origin": "https://example.com", "fcm_sender_id": "123"})
	srv.AddApp("Another", nil)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig(srv) + fmt.Sprintf(`
data "onesignal_app" "d" {
  id = %q
}

data "onesignal_apps" "all" {}
`, appID),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.onesignal_app.d", "name", "Doppelganger"),
				resource.TestCheckResourceAttr("data.onesignal_app.d", "fcm_configured", "true"),
				resource.TestCheckResourceAttr("data.onesignal_app.d", "apns_configured", "false"),
				resource.TestCheckResourceAttr("data.onesignal_apps.all", "apps.#", "2"),
				resource.TestCheckResourceAttr("data.onesignal_apps.all", "apps.0.name", "Another"),
			),
		}},
	})
}
