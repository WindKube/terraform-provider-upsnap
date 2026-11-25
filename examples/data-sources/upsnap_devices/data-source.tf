data "upsnap_devices" "online_lab" {
  status = "online"
  filter = "ip ~ \"192.168.10.\""
}

output "online_lab_devices" {
  value = data.upsnap_devices.online_lab.devices[*].name
}
