# OpenTofu provider for OneSignal

Manages the configuration of [OneSignal](https://onesignal.com) apps through
OneSignal's REST API:

| Resource                | What it manages                                                                 |
| ----------------------- | ------------------------------------------------------------------------------- |
| `onesignal_app`         | An app's settings and its push credentials, which are write-only                 |
| `onesignal_app_api_key` | An app's API keys, with their IP allowlists and in-place rotation               |
| `onesignal_segment`     | Filter-based segments                                                           |
| `onesignal_template`    | Push, email and SMS templates                                                   |

The data sources `onesignal_app` and `onesignal_apps` read one app or list
them all.

Not covered: Journeys, whose API is in beta, and everything that is not
configuration, such as users, subscriptions, messages and analytics. OneSignal
has no API for deleting an app, so destroying an `onesignal_app` fails; delete
the app in the dashboard, then remove it from state.

The documentation for each resource is in [`docs/`](docs/).

## Requirements

- [OpenTofu](https://opentofu.org) 1.11 or later, the first release with
  write-only attributes. Tested with 1.13.
- A OneSignal Organization API key.

## Installation

The provider is not on a public registry. Builds are served from the mirror at
`https://opentofu.regulad.xyz/`, under the address
`registry.opentofu.org/regulad/onesignal`; see [`docs/index.md`](docs/index.md)
for the CLI configuration that reaches it.

## Development

Requires Go (the version in [`go.mod`](go.mod)), OpenTofu, `jq`, and
`golangci-lint` for `make lint`.

| Command              | What it does                                                                                     |
| -------------------- | ------------------------------------------------------------------------------------------------ |
| `make build`         | Builds the provider                                                                              |
| `make test`          | Runs the client's unit tests                                                                     |
| `make testacc`       | Also runs the resource tests, which drive OpenTofu against an in-memory fake of OneSignal's API |
| `make generate`      | Formats the examples and regenerates `docs/` from them, `templates/` and the schema              |
| `make docs-validate` | Checks `docs/` with tfplugindocs                                                                 |

Nothing in the test suite reaches OneSignal. The fake, in
[`internal/onesignal/fake`](internal/onesignal/fake), follows the API's
documented behaviour, quirks included.

The docs are generated from the schema as OpenTofu reports it
([`tools/docs.sh`](tools/docs.sh)), so no Terraform binary is needed.

## Licence

[MPL-2.0](LICENSE).
