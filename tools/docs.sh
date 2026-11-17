#!/bin/sh
# SPDX-License-Identifier: MPL-2.0
#
# Runs tfplugindocs (generate or validate) on the provider's schema as OpenTofu
# reports it, so that no Terraform binary is needed. Usage: docs.sh generate|validate
set -eu

cd "$(dirname "$0")"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin" "$tmp/config"

(cd .. && go build -o "$tmp/bin/terraform-provider-onesignal" .)
cat > "$tmp/tofurc" <<EOF
provider_installation {
  dev_overrides {
    "registry.opentofu.org/regulad/onesignal" = "$tmp/bin"
  }
  direct {}
}
EOF
cat > "$tmp/config/main.tf" <<'EOF'
terraform {
  required_providers {
    onesignal = { source = "regulad/onesignal" }
  }
}
EOF

# A development override needs no init. tfplugindocs finds a schema only under
# the provider's short name, so the address is shortened to it.
(cd "$tmp/config" && TF_CLI_CONFIG_FILE="$tmp/tofurc" tofu providers schema -json) |
	jq '.provider_schemas |= with_entries(.key |= sub(".*/"; ""))' > "$tmp/schema.json"

tfplugindocs() {
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs "$@"
}
case "${1:-}" in
generate)
	tfplugindocs generate --provider-dir .. --provider-name onesignal \
		--rendered-provider-name OneSignal --providers-schema "$tmp/schema.json"
	;;
validate)
	tfplugindocs validate --provider-dir .. --provider-name onesignal \
		--providers-schema "$tmp/schema.json"
	;;
*)
	echo "usage: $0 generate|validate" >&2
	exit 2
	;;
esac
