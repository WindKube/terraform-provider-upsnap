data "upsnap_group" "office" {
  name = "Office"
}

output "office_device_ids" {
  value = data.upsnap_group.office.device_ids
}
