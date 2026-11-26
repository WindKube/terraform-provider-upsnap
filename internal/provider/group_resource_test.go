package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccGroupConfig(name string) string {
	return fmt.Sprintf(`
resource "upsnap_group" "test" {
  name = %q
}
`, name)
}

func TestAccGroupResource(t *testing.T) {
	name := testAccName("group")
	address := "upsnap_group.test"
	var id string

	steps := []resource.TestStep{
		{
			Config: testAccGroupConfig(name),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name)),
			},
			Check: testAccCaptureID(address, &id),
		},
	}
	steps = append(steps, testAccImportSteps(address)...)
	steps = append(steps, resource.TestStep{
		Config: testAccGroupConfig(name + "-renamed"),
		ConfigStateChecks: []statecheck.StateCheck{
			statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name+"-renamed")),
		},
		Check: resource.TestCheckResourceAttrPtr(address, "id", &id),
	})
	steps = append(steps, testAccDriftDeletedSteps(testAccGroupConfig(name), func() {
		if err := testAccClient(t).Groups.Delete(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	})...)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func TestAccGroupResource_identity(t *testing.T) {
	testAccIdentityTest(t, "upsnap_group.test", testAccGroupConfig(testAccName("group")))
}

func testAccGroupMembershipConfig(name string, groups string) string {
	return fmt.Sprintf(`
resource "upsnap_group" "office" {
  name = "%[1]s-office"
}

resource "upsnap_group" "lab" {
  name = "%[1]s-lab"
}

resource "upsnap_device" "test" {
  name      = %[1]q
  ip        = "192.0.2.41"
  mac       = "aa:bb:cc:dd:ee:41"
  group_ids = %[2]s
}

data "upsnap_group" "office" {
  name       = upsnap_group.office.name
  depends_on = [upsnap_device.test]
}

data "upsnap_groups" "all" {
  depends_on = [upsnap_group.office, upsnap_group.lab]
}

data "upsnap_devices" "in_lab" {
  group_id   = upsnap_group.lab.id
  depends_on = [upsnap_device.test]
}
`, name, groups)
}

func TestAccGroupMembership(t *testing.T) {
	name := testAccName("member")
	device := "upsnap_device.test"
	var groupID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGroupMembershipConfig(name, "[upsnap_group.office.id]"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(device, tfjsonpath.New("group_ids"), knownvalue.SetSizeExact(1)),
					statecheck.CompareValuePairs("data.upsnap_group.office", tfjsonpath.New("id"), "upsnap_group.office", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.upsnap_group.office", tfjsonpath.New("device_ids"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue("data.upsnap_devices.in_lab", tfjsonpath.New("devices"), knownvalue.ListSizeExact(0)),
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemAttrPair(device, "group_ids.*", "upsnap_group.office", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.upsnap_group.office", "device_ids.*", device, "id"),
					testAccCaptureID("upsnap_group.office", &groupID),
				),
			},
			{
				Config: testAccGroupMembershipConfig(name, "[upsnap_group.office.id, upsnap_group.lab.id]"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(device, tfjsonpath.New("group_ids"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue("data.upsnap_devices.in_lab", tfjsonpath.New("devices"), knownvalue.ListSizeExact(1)),
				},
			},
			{
				Config: testAccGroupMembershipConfig(name, "[upsnap_group.lab.id]"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(device, tfjsonpath.New("group_ids"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue("data.upsnap_group.office", tfjsonpath.New("device_ids"), knownvalue.SetSizeExact(0)),
				},
				Check: resource.TestCheckTypeSetElemAttrPair(device, "group_ids.*", "upsnap_group.lab", "id"),
			},
			{
				Config: testAccGroupMembershipConfig(name, "[]"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(device, tfjsonpath.New("group_ids"), knownvalue.SetSizeExact(0)),
				},
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					d, err := c.Devices.FindOne(t.Context(), `name = "`+name+`"`)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := c.Devices.Update(t.Context(), d.ID, map[string]any{"groups+": groupID}); err != nil {
						t.Fatal(err)
					}
				},
				Config:             testAccGroupMembershipConfig(name, "[]"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
