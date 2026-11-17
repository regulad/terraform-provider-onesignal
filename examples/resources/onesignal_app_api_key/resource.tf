resource "onesignal_app_api_key" "server" {
  app_id = onesignal_app.example.id
  name   = "server"

  # Only these addresses may use the key. Leave it empty to allow any.
  ip_allowlist = ["192.0.2.10/32", "2001:db8::10/128"]

  # Changing any value here rotates the key in place.
  rotate_when_changed = {
    rotation = "2026-11"
  }
}

output "server_api_key" {
  value     = onesignal_app_api_key.server.value
  sensitive = true
}
