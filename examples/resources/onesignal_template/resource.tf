resource "onesignal_template" "welcome" {
  app_id         = onesignal_app.example.id
  api_key        = onesignal_app_api_key.server.value
  name           = "Welcome"
  channel        = "push"
  headings       = { en = "Welcome" }
  contents       = { en = "Thanks for signing up.", es = "Gracias por registrarte." }
  url            = "https://example.com/welcome"
  push_platforms = ["ios", "android", "chrome_web"]
}

resource "onesignal_template" "receipt" {
  app_id             = onesignal_app.example.id
  api_key            = onesignal_app_api_key.server.value
  name               = "Receipt"
  channel            = "email"
  email_subject      = "Your receipt"
  email_body         = file("receipt.html")
  email_from_name    = "Example"
  email_from_address = "receipts@example.com"
}

resource "onesignal_template" "code" {
  app_id   = onesignal_app.example.id
  api_key  = onesignal_app_api_key.server.value
  name     = "Sign-in code"
  channel  = "sms"
  contents = { en = "Your sign-in code is {{ code }}." }
}
