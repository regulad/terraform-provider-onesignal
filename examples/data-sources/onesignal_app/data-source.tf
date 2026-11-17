data "onesignal_app" "example" {
  id = "00000000-0000-0000-0000-000000000000"
}

output "web_push_configured" {
  value = data.onesignal_app.example.web_configured
}
