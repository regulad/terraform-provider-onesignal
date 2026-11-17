// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

// The live tests run against OneSignal itself, with ONESIGNAL_LIVE=1, TF_ACC=1
// and an Organization API key in ONESIGNAL_ORGANIZATION_API_KEY. They work in
// one app, named liveAppName, which must exist: OneSignal cannot delete apps,
// so the tests never create one. Segments are left out, as OneSignal refuses to
// create them on the Free plan.
const liveAppName = "homelab-provider-test"

// liveApp returns the test app, skipping the test unless live tests are on.
func liveApp(t *testing.T) *onesignal.App {
	t.Helper()
	if os.Getenv("ONESIGNAL_LIVE") != "1" {
		t.Skip("ONESIGNAL_LIVE is not 1")
	}
	c := onesignal.New(onesignal.DefaultBaseURL, os.Getenv(EnvOrganizationAPIKey), "terraform-provider-onesignal/test")
	apps, err := c.ListApps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range apps {
		if apps[i].Name == liveAppName {
			return &apps[i]
		}
	}
	t.Fatalf("no app named %s", liveAppName)
	return nil
}

func TestLiveAppAPIKey(t *testing.T) {
	app := liveApp(t)
	cfg := func(allowlist, rotation string) string {
		return fmt.Sprintf(`
resource "onesignal_app_api_key" "test" {
  app_id              = %q
  name                = "provider-live-test"
  ip_allowlist        = %s
  rotate_when_changed = { at = %q }
}
`, app.ID, allowlist, rotation)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`["192.0.2.10/32", "2001:db8::10/128"]`, "1"),
				Check:  resource.TestCheckResourceAttr("onesignal_app_api_key.test", "ip_allowlist.#", "2"),
			},
			{Config: cfg(`[]`, "2")},
		},
	})
}

func TestLiveTemplates(t *testing.T) {
	app := liveApp(t)
	cfg := func(headings, platforms, fromName string) string {
		return fmt.Sprintf(`
resource "onesignal_app_api_key" "k" {
  app_id = %[1]q
  name   = "provider-live-test-templates"
}

resource "onesignal_template" "push" {
  app_id         = %[1]q
  api_key        = onesignal_app_api_key.k.value
  name           = "provider-live-test-push"
  channel        = "push"
  contents       = { en = "Hello", es = "Hola" }
  headings       = %[2]s
  subtitle       = { en = "Sub" }
  url            = "https://example.com/welcome"
  global_image   = "https://example.com/welcome.png"
  push_platforms = %[3]s
}

resource "onesignal_template" "web" {
  app_id   = %[1]q
  api_key  = onesignal_app_api_key.k.value
  name     = "provider-live-test-web"
  channel  = "push"
  contents = { en = "Hello" }
}

resource "onesignal_template" "email" {
  app_id             = %[1]q
  api_key            = onesignal_app_api_key.k.value
  name               = "provider-live-test-email"
  channel            = "email"
  email_subject      = "Subject"
  email_body         = "<p>Body</p>"
  email_preheader    = "Preheader"
  email_from_name    = %[4]s
  email_from_address = "from@example.com"
}

resource "onesignal_template" "sms" {
  app_id         = %[1]q
  api_key        = onesignal_app_api_key.k.value
  name           = "provider-live-test-sms"
  channel        = "sms"
  contents       = { en = "Code 123" }
  sms_from       = "+15555550100"
  sms_media_urls = ["https://example.com/m.png"]
}
`, app.ID, headings, platforms, fromName)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(`{ en = "Welcome" }`, `["ios", "android"]`, `"Example"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("onesignal_template.push", "push_platforms.#", "2"),
					resource.TestCheckResourceAttr("onesignal_template.web", "push_platforms.#", "3"),
					resource.TestCheckResourceAttr("onesignal_template.sms", "channel", "sms"),
				),
			},
			{
				Config: cfg(`null`, `["ios", "safari"]`, `null`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr("onesignal_template.push", "headings.%"),
					resource.TestCheckResourceAttr("onesignal_template.push", "push_platforms.#", "2"),
					resource.TestCheckTypeSetElemAttr("onesignal_template.push", "push_platforms.*", "safari"),
					resource.TestCheckNoResourceAttr("onesignal_template.email", "email_from_name"),
				),
			},
		},
	})
}

// TestLiveAppAndDataSources imports the test app, expecting no change to plan,
// and leaves it in OneSignal when done.
func TestLiveAppAndDataSources(t *testing.T) {
	app := liveApp(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
import {
  to = onesignal_app.test
  id = %[1]q
}

resource "onesignal_app" "test" {
  name              = %[2]q
  organization_id   = %[3]q
  chrome_web_origin = %[4]q
}

data "onesignal_app" "test" {
  id = %[1]q
}

data "onesignal_apps" "all" {}
`, app.ID, app.Name, app.OrganizationID, app.ChromeWebOrigin),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("onesignal_app.test", plancheck.ResourceActionNoop)},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.onesignal_app.test", "name", liveAppName),
					resource.TestCheckResourceAttr("data.onesignal_app.test", "web_configured", "true"),
					resource.TestCheckResourceAttrSet("onesignal_app.test", "chrome_web_key"),
				),
			},
			{
				Config: `
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
