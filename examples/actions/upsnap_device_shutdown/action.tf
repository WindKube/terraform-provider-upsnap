# terraform apply -invoke=action.upsnap_device_shutdown.nas
action "upsnap_device_shutdown" "nas" {
  config {
    device_id = upsnap_device.nas.id
  }
}
