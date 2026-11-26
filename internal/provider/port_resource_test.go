package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

func TestAccPortResource_survivesPingJob(t *testing.T) {
	name := testAccName("port")
	config := func(device string) string {
		return fmt.Sprintf(`
resource "upsnap_device" "a" {
  name     = "%[1]s-a"
  ip       = "192.0.2.33"
  mac      = "aa:bb:cc:dd:ee:33"
  ping_cmd = %[3]q
}

resource "upsnap_device" "b" {
  name     = "%[1]s-b"
  ip       = "192.0.2.34"
  mac      = "aa:bb:cc:dd:ee:34"
  ping_cmd = %[4]q
}

resource "upsnap_port" "test" {
  device_id = upsnap_device.%[2]s.id
  name      = %[1]q
  number    = 22
}
`, name, device, testAccRacingPingCmd(name+"-a"), testAccRacingPingCmd(name+"-b"))
	}
	var steps []resource.TestStep
	for _, device := range []string{"a", "b", "a", "b"} {
		steps = append(steps, resource.TestStep{
			Config: config(device),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair("upsnap_port.test", "device_id", "upsnap_device."+device, "id"),
				testAccCheckPortLinkStays(t),
			),
		})
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t); testAccRacingPingJob(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// testAccCheckPortLinkStays waits for UpSnap's ping job and checks that only the device in the
// port's state still lists the port.
func testAccCheckPortLinkStays(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		time.Sleep(4 * time.Second)
		port := s.RootModule().Resources["upsnap_port.test"].Primary
		c := testAccClient(t)
		for _, address := range []string{"upsnap_device.a", "upsnap_device.b"} {
			id := s.RootModule().Resources[address].Primary.ID
			d, err := c.Devices.Get(context.Background(), id)
			if err != nil {
				return err
			}
			want := id == port.Attributes["device_id"]
			if got := slices.Contains(d.Ports, port.ID); got != want {
				return fmt.Errorf("UpSnap reverted the ports of %s: has port = %t, want %t", address, got, want)
			}
		}
		return nil
	}
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
