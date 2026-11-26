resource "upsnap_settings" "this" {
  interval          = "*/5 * * * * *"
  scan_range        = "192.168.1.0/24"
  lazy_ping         = true
  track_ip_interval = "0 */10 * * * *"
  website_title     = "Homelab"
}
