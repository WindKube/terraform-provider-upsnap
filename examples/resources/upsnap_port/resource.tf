resource "upsnap_device" "nas" {
  name = "NAS"
  ip   = "192.168.1.20"
  mac  = "aa:bb:cc:dd:ee:ff"
}

resource "upsnap_port" "nas_web" {
  device_id = upsnap_device.nas.id
  name      = "DSM"
  number    = 5000
  link      = "http://192.168.1.20:5000"
}

resource "upsnap_port" "nas_ssh" {
  device_id = upsnap_device.nas.id
  name      = "SSH"
  number    = 22
}
