package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// UpSnap treats a ping_cmd exit code of 0 as online, so ping_cmd = "true" makes a wake succeed
// as soon as the magic packet is sent and ping_cmd = "false" makes a shutdown with a no-op
// command succeed at once, whatever the network around the container answers.
func testAccActionConfig(name, trigger, action string) string {
	return fmt.Sprintf(`
resource "upsnap_group" "test" {
  name = %[1]q
}

resource "upsnap_device" "up" {
  name         = "%[1]s-up"
  ip           = "192.0.2.61"
  mac          = "aa:bb:cc:dd:ee:61"
  ping_cmd     = "true"
  wake_timeout = 20
  group_ids    = [upsnap_group.test.id]
}

resource "upsnap_device" "down" {
  name             = "%[1]s-down"
  ip               = "192.0.2.62"
  mac              = "aa:bb:cc:dd:ee:62"
  ping_cmd         = "false"
  shutdown_cmd     = "true"
  shutdown_timeout = 20
}

resource "terraform_data" "trigger" {
  input = %[2]q

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.%[3]s.test]
    }
  }
}

%[4]s
`, name, trigger, actionType(action), action)
}

func actionType(block string) string {
	return regexp.MustCompile(`action "([a-z_]+)"`).FindStringSubmatch(block)[1]
}

func testAccCheckDeviceStatus(t *testing.T, name, want string) func() {
	return func() {
		device, err := testAccClient(t).Devices.FindOne(t.Context(), client.Eq("name", name))
		if err != nil {
			t.Fatal(err)
		}
		if device.Status != want {
			t.Errorf("device %s status is %q, want %q", name, device.Status, want)
		}
	}
}

func TestAccDevicePowerActions(t *testing.T) {
	testAccSkipOpenTofu(t)
	name := testAccName("action")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		Steps: []resource.TestStep{
			{
				Config: testAccActionConfig(name, "wake-1", `
action "upsnap_device_wake" "test" {
  config {
    device_id = upsnap_device.up.id
    timeout   = "2m"
  }
}
`),
				PostApplyFunc: testAccCheckDeviceStatus(t, name+"-up", "online"),
			},
			{
				Config: testAccActionConfig(name, "shutdown-1", `
action "upsnap_device_shutdown" "test" {
  config {
    device_id = upsnap_device.down.id
  }
}
`),
				PostApplyFunc: testAccCheckDeviceStatus(t, name+"-down", "offline"),
			},
			{
				Config: testAccActionConfig(name, "wake-async", `
action "upsnap_device_wake" "test" {
  config {
    device_id = upsnap_device.up.id
    wait      = false
  }
}
`),
			},
			{
				Config: testAccActionConfig(name, "sleep-1", `
action "upsnap_device_sleep" "test" {
  config {
    device_id = upsnap_device.up.id
  }
}
`),
				ExpectError: regexp.MustCompile(`Unable to sleep device`),
			},
			{
				Config: testAccActionConfig(name, "missing-1", `
action "upsnap_device_wake" "test" {
  config {
    device_id = "zzzzzzzzzzzzzzz"
  }
}
`),
				ExpectError: regexp.MustCompile(`Device not found`),
			},
			{
				Config: testAccActionConfig(name, "group-1", `
action "upsnap_group_shutdown" "test" {
  config {
    group_id = upsnap_group.test.id
  }
}
`),
			},
			{
				Config: testAccActionConfig(name, "group-2", `
action "upsnap_group_wake" "test" {
  config {
    group_id = upsnap_group.test.id
  }
}
`),
			},
		},
	})
}

func TestAccDevicePowerActions_validation(t *testing.T) {
	testAccSkipOpenTofu(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		Steps: []resource.TestStep{{
			Config: `
provider "upsnap" {
  endpoint = "http://127.0.0.1:1"
  token    = "a.b.c"
}

action "upsnap_device_reboot" "test" {
  config {
    device_id = "z5lghx2r3tm45n1"
    timeout   = "soon"
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.upsnap_device_reboot.test]
    }
  }
}
`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`is not a positive Go duration`),
		}},
	})
}
