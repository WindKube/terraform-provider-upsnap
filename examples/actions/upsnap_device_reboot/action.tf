# terraform apply -invoke=action.upsnap_device_reboot.nas
action "upsnap_device_reboot" "nas" {
  config {
    device_id = upsnap_device.nas.id
    wait      = false
  }
}
