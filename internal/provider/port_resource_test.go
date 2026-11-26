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

func testAccPortConfig(name, device string, number int, link string) string {
	return fmt.Sprintf(`
resource "upsnap_device" "a" {
  name = "%[1]s-a"
  ip   = "192.0.2.31"
  mac  = "aa:bb:cc:dd:ee:31"
}

resource "upsnap_device" "b" {
  name = "%[1]s-b"
  ip   = "192.0.2.32"
  mac  = "aa:bb:cc:dd:ee:32"
}

resource "upsnap_port" "test" {
  device_id = upsnap_device.%[2]s.id
  name      = "%[1]s"
  number    = %[3]d
  link      = %[4]q
}
`, name, device, number, link)
}

func TestAccPortResource(t *testing.T) {
	name := testAccName("port")
	address := "upsnap_port.test"
	var id, deviceA string

	steps := []resource.TestStep{
		{
			Config: testAccPortConfig(name, "a", 22, ""),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("number"), knownvalue.Int64Exact(22)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("link"), knownvalue.StringExact("")),
				statecheck.CompareValuePairs(address, tfjsonpath.New("device_id"), "upsnap_device.a", tfjsonpath.New("id"), compare.ValuesSame()),
			},
			Check: resource.ComposeTestCheckFunc(
				testAccCaptureID(address, &id),
				testAccCaptureID("upsnap_device.a", &deviceA),
			),
		},
		{
			RefreshState: true,
			Check:        resource.TestCheckTypeSetElemAttrPair("upsnap_device.a", "port_ids.*", address, "id"),
		},
	}
	steps = append(steps, testAccImportSteps(address)...)
	steps = append(steps,
		resource.TestStep{
			Config: testAccPortConfig(name, "b", 443, "https://example.com"),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("number"), knownvalue.Int64Exact(443)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("link"), knownvalue.StringExact("https://example.com")),
				statecheck.CompareValuePairs(address, tfjsonpath.New("device_id"), "upsnap_device.b", tfjsonpath.New("id"), compare.ValuesSame()),
			},
			Check: resource.TestCheckResourceAttrPtr(address, "id", &id),
		},
		resource.TestStep{
			RefreshState: true,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("upsnap_device.a", "port_ids.#", "0"),
				resource.TestCheckTypeSetElemAttrPair("upsnap_device.b", "port_ids.*", address, "id"),
			),
		},
		resource.TestStep{
			PreConfig: func() {
				c := testAccClient(t)
				device, err := c.Devices.FindOne(t.Context(), `ports.id ?= "`+id+`"`)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := c.Devices.Update(t.Context(), device.ID, map[string]any{"ports-": id}); err != nil {
					t.Fatal(err)
				}
			},
			Config:             testAccPortConfig(name, "b", 443, "https://example.com"),
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			Config: testAccPortConfig(name, "b", 443, "https://example.com"),
			Check:  resource.TestCheckResourceAttrPtr(address, "id", &id),
		},
		resource.TestStep{
			RefreshState: true,
			Check:        resource.TestCheckTypeSetElemAttrPair("upsnap_device.b", "port_ids.*", address, "id"),
		},
	)
	steps = append(steps, testAccDriftDeletedSteps(testAccPortConfig(name, "b", 443, "https://example.com"), func() {
		if err := testAccClient(t).Ports.Delete(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	})...)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func TestAccPortResource_identity(t *testing.T) {
	testAccIdentityTest(t, "upsnap_port.test", testAccPortConfig(testAccName("port"), "a", 8080, ""))
}

func TestAccPortResource_validation(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
provider "upsnap" {
  endpoint = "http://127.0.0.1:1"
  token    = "a.b.c"
}

resource "upsnap_port" "test" {
  device_id = "z5lghx2r3tm45n1"
  name      = "bad"
  number    = 0
}
`,
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(`between 1 and 65535`),
		}},
	})
}
