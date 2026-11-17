// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal/fake"
)

// The resource tests drive OpenTofu, whose implied provider address is on
// registry.opentofu.org, against the fake API. They run under TF_ACC=1.
func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		if tofu, err := exec.LookPath("tofu"); err == nil {
			_ = os.Setenv("TF_ACC_TERRAFORM_PATH", tofu)
		}
	}
	if os.Getenv("TF_ACC_PROVIDER_HOST") == "" {
		_ = os.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	}
	os.Exit(m.Run())
}

var testFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"onesignal": providerserver.NewProtocol6WithError(New("test")()),
}

// providerConfig points the provider at srv.
func providerConfig(srv *fake.Server) string {
	return fmt.Sprintf(`
provider "onesignal" {
  base_url             = %q
  organization_api_key = %q
}
`, srv.URL, srv.OrganizationKey)
}

// stateAttr returns a resource attribute from state.
func stateAttr(s *terraform.State, address, name string) (string, error) {
	rs, ok := s.RootModule().Resources[address]
	if !ok {
		return "", fmt.Errorf("%s is not in state", address)
	}
	return rs.Primary.Attributes[name], nil
}
