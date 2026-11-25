resource "upsnap_user" "alice" {
  username = "alice"
  password = var.alice_password
}

resource "upsnap_device" "workstation" {
  name = "Workstation"
  ip   = "192.168.1.30"
  mac  = "aa:bb:cc:dd:ee:30"
}

resource "upsnap_permission" "alice" {
  user_id = upsnap_user.alice.id
  read    = [upsnap_device.workstation.id]
  power   = [upsnap_device.workstation.id]
}

variable "alice_password" {
  type      = string
  sensitive = true
}
