data "upsnap_device" "nas" {
  name = "NAS"
}

output "nas_status" {
  value = data.upsnap_device.nas.status
}
