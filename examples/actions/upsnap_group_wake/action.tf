# terraform apply -invoke=action.upsnap_group_wake.office
action "upsnap_group_wake" "office" {
  config {
    group_id = upsnap_group.office.id
  }
}
