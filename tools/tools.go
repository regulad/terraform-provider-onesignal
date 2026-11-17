// SPDX-License-Identifier: MPL-2.0

//go:build generate

package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

// Format the examples, then generate the documentation from them and the schema.
//go:generate tofu fmt -recursive ../examples/
//go:generate sh docs.sh generate
