data "upsnap_groups" "all" {}

output "group_names" {
  value = data.upsnap_groups.all.groups[*].name
}
