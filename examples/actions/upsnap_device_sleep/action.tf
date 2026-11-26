# terraform apply -invoke=action.upsnap_device_sleep.workstation
action "upsnap_device_sleep" "workstation" {
  config {
    device_id = upsnap_device.workstation.id
  }
}
