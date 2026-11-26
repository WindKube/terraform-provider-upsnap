resource "upsnap_group" "office" {
  name = "Office"
}

resource "upsnap_device" "workstation" {
  name      = "Workstation"
  ip        = "192.168.1.30"
  mac       = "aa:bb:cc:dd:ee:30"
  group_ids = [upsnap_group.office.id]
}
