terraform {
  required_providers {
    onesignal = {
      source = "regulad/onesignal"
    }
  }
}

# The Organization API key is read from ONESIGNAL_ORGANIZATION_API_KEY.
provider "onesignal" {}
