resource "upsnap_user" "alice" {
  username = "alice"
  email    = "alice@example.com"
  password = var.alice_password
  verified = true
}

variable "alice_password" {
  type      = string
  sensitive = true
}
