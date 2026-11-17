data "onesignal_apps" "all" {}

output "app_ids_by_name" {
  value = { for app in data.onesignal_apps.all.apps : app.name => app.id }
}
