resource "onesignal_segment" "engaged" {
  app_id      = onesignal_app.example.id
  api_key     = onesignal_app_api_key.server.value
  name        = "Engaged"
  description = "Frequent users in the US, or anyone on the pro plan"

  # Adjacent filters are joined by AND; an OR operator separates groups.
  filters = [
    { field = "session_count", relation = ">", value = "5" },
    { field = "country", relation = "=", value = "US" },
    { operator = "OR" },
    { field = "tag", key = "plan", relation = "=", value = "pro" },
  ]
}
