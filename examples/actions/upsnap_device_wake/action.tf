action "upsnap_device_wake" "nas" {
  config {
    device_id = upsnap_device.nas.id
    timeout   = "3m"
  }
}

# Wake the NAS before a backup job is (re)deployed. Run it ad hoc with:
#   terraform apply -invoke=action.upsnap_device_wake.nas
resource "terraform_data" "backup_job" {
  input = var.backup_job_version

  lifecycle {
    action_trigger {
      events  = [before_create, before_update]
      actions = [action.upsnap_device_wake.nas]
    }
  }
}

variable "backup_job_version" {
  type = string
}
