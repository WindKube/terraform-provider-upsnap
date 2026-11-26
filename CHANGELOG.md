## 0.1.0 (Unreleased)

FEATURES:

* **New Resource:** `upsnap_device`
* **New Resource:** `upsnap_port`
* **New Resource:** `upsnap_group`
* **New Resource:** `upsnap_user`
* **New Resource:** `upsnap_permission`
* **New Resource:** `upsnap_settings`
* **New Data Source:** `upsnap_device`
* **New Data Source:** `upsnap_devices`
* **New Data Source:** `upsnap_group`
* **New Data Source:** `upsnap_groups`
* **New Action:** `upsnap_device_wake`, `upsnap_device_shutdown`, `upsnap_device_sleep`, `upsnap_device_reboot`
* **New Action:** `upsnap_group_wake`, `upsnap_group_shutdown`
* All resources support import by ID, import blocks and resource identity (Terraform 1.12+).

NOTES:

* UpSnap 5.7.3's ping job can revert device and port changes made while it runs. The provider watches each change for
  `settle_time` (default `5s`) and writes it again when UpSnap reverts it; see the provider documentation's known issues.
