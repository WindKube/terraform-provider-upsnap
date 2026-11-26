# terraform apply -invoke=action.upsnap_group_shutdown.office
action "upsnap_group_shutdown" "office" {
  config {
    group_id = upsnap_group.office.id
  }
}
