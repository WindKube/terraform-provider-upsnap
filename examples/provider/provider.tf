terraform {
  required_providers {
    upsnap = {
      source = "windkube/upsnap"
    }
  }
}

# Credentials can also come from UPSNAP_ENDPOINT, UPSNAP_USERNAME and UPSNAP_PASSWORD (or UPSNAP_TOKEN).
provider "upsnap" {
  endpoint = "http://upsnap.lan:8090"
  username = "admin@example.com"
  password = var.upsnap_password
}

variable "upsnap_password" {
  type      = string
  sensitive = true
}
