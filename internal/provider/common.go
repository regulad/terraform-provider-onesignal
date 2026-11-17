// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/regulad/terraform-provider-onesignal/internal/onesignal"
)

// clientFrom returns the client a resource or data source was configured with.
func clientFrom(data any, diags *diag.Diagnostics) *onesignal.Client {
	if data == nil {
		return nil // The provider is not configured yet.
	}
	c, ok := data.(*onesignal.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *onesignal.Client, got %T.", data))
		return nil
	}
	return c
}

// splitImportID splits "<app_id>/<id>".
func splitImportID(id, what string, diags *diag.Diagnostics) (string, string, bool) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		diags.AddError("Invalid import ID", fmt.Sprintf("Import a %s as <app_id>/<%s_id>; got %q.", what, what, id))
		return "", "", false
	}
	return parts[0], parts[1], true
}
