resource "upsnap_device" "nas" {
  name        = "NAS"
  ip          = "192.168.1.20"
  mac         = "aa:bb:cc:dd:ee:ff"
  netmask     = "255.255.255.0"
  description = "Synology in the basement"
  link        = "http://192.168.1.20:5000"
  link_open   = "new_tab"

  shutdown_cmd = "sshpass -p \"$NAS_PASSWORD\" ssh -o StrictHostKeyChecking=no admin@{{ DEVICE_IP }} 'sudo poweroff'"

  wake_cron             = "0 7 * * 1-5"
  wake_cron_enabled     = true
  shutdown_cron         = "0 23 * * *"
  shutdown_cron_enabled = true
}
