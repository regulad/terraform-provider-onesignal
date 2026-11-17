resource "onesignal_app" "example" {
  name              = "Example"
  organization_id   = "00000000-0000-0000-0000-000000000000"
  chrome_web_origin = "https://example.com"

  apns_env       = "production"
  apns_key_id    = "ABCDE12345"
  apns_team_id   = "FGHIJ67890"
  apns_bundle_id = "com.example.app"

  # Credentials are write-only: they are sent when created, and again whenever
  # their version changes, and never stored in state.
  apns_p8_wo         = file("AuthKey_ABCDE12345.p8")
  apns_p8_wo_version = 1

  fcm_v1_service_account_json_wo         = file("firebase-service-account.json")
  fcm_v1_service_account_json_wo_version = 1
}
