package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccDeviceDataSources(t *testing.T) {
	prefix := testAccName("ds")
	config := fmt.Sprintf(`
resource "upsnap_device" "a" {
  name = "%[1]s-a"
  ip   = "192.0.2.21"
  mac  = "aa:bb:cc:dd:ee:21"
}

resource "upsnap_device" "b" {
  name = "%[1]s-b"
  ip   = "192.0.2.22"
  mac  = "aa:bb:cc:dd:ee:22"
}

resource "upsnap_device" "dup1" {
  name = "%[1]s-dup"
  ip   = "192.0.2.23"
  mac  = "aa:bb:cc:dd:ee:23"
}

resource "upsnap_device" "dup2" {
  name = "%[1]s-dup"
  ip   = "192.0.2.24"
  mac  = "aa:bb:cc:dd:ee:24"
}

data "upsnap_device" "by_id" {
  id = upsnap_device.a.id
}

data "upsnap_device" "by_name" {
  name = upsnap_device.b.name
}

data "upsnap_devices" "prefix" {
  filter     = "name ~ \"%[1]s\""
  depends_on = [upsnap_device.a, upsnap_device.b, upsnap_device.dup1, upsnap_device.dup2]
}

data "upsnap_devices" "exact" {
  name       = "%[1]s-dup"
  depends_on = [upsnap_device.dup1, upsnap_device.dup2]
}
`, prefix)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("data.upsnap_device.by_id", tfjsonpath.New("name"), "upsnap_device.a", tfjsonpath.New("name"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.upsnap_device.by_id", tfjsonpath.New("mac"), knownvalue.StringExact("aa:bb:cc:dd:ee:21")),
					statecheck.CompareValuePairs("data.upsnap_device.by_name", tfjsonpath.New("id"), "upsnap_device.b", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue("data.upsnap_devices.prefix", tfjsonpath.New("devices"), knownvalue.ListSizeExact(4)),
					statecheck.ExpectKnownValue("data.upsnap_devices.prefix", tfjsonpath.New("devices").AtSliceIndex(0).AtMapKey("name"), knownvalue.StringExact(prefix+"-a")),
					statecheck.ExpectKnownValue("data.upsnap_devices.exact", tfjsonpath.New("devices"), knownvalue.ListSizeExact(2)),
				},
			},
			{
				Config: config + fmt.Sprintf(`
data "upsnap_device" "ambiguous" {
  name = "%s-dup"
}
`, prefix),
				ExpectError: regexp.MustCompile(`Device name is not unique`),
			},
			{
				Config: config + `
data "upsnap_device" "missing" {
  id = "zzzzzzzzzzzzzzz"
}
`,
				ExpectError: regexp.MustCompile(`Device not found`),
			},
		},
	})
}
