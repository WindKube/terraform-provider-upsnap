package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccDeviceConfigBasic(name string) string {
	return fmt.Sprintf(`
resource "upsnap_device" "test" {
  name = %q
  ip   = "192.0.2.10"
  mac  = "aa:bb:cc:dd:ee:01"
}
`, name)
}

func testAccDeviceConfigFull(name string) string {
	return fmt.Sprintf(`
resource "upsnap_device" "test" {
  name                  = %q
  ip                    = "nas.example.com"
  mac                   = "AA-BB-CC-DD-EE-02"
  netmask               = "255.255.255.0"
  description           = "managed by terraform"
  link                  = "http://nas.example.com:5000"
  link_open             = "new_tab"
  track_ip              = true
  wol_password          = "abcd"
  wake_cmd              = "echo wake {{ DEVICE_MAC }}"
  wake_confirm          = true
  wake_timeout          = 90
  wake_cron             = "0 7 * * 1-5"
  wake_cron_enabled     = true
  shutdown_cmd          = "echo shutdown {{ DEVICE_IP }}"
  shutdown_confirm      = true
  shutdown_timeout      = 60
  shutdown_cron         = "0 0 23 * * *"
  shutdown_cron_enabled = true
  ping_cmd              = "true"
  sol_enabled           = true
  sol_port              = 8009
  sol_auth              = true
  sol_user              = "sol"
  sol_password          = "sol-secret"
}
`, name)
}

func TestAccDeviceResource(t *testing.T) {
	name := testAccName("device")
	var id string
	address := "upsnap_device.test"

	steps := []resource.TestStep{
		{
			Config: testAccDeviceConfigBasic(name),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("id"), knownvalue.StringRegexp(regexp.MustCompile(`^[a-z0-9]{15}$`))),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("netmask"), knownvalue.StringExact("255.255.255.255")),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("wake_timeout"), knownvalue.Null()),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("group_ids"), knownvalue.SetSizeExact(0)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("port_ids"), knownvalue.SetSizeExact(0)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("created_by"), knownvalue.StringExact("")),
			},
			Check: testAccCaptureID(address, &id),
		},
	}
	steps = append(steps, testAccImportSteps(address)...)
	steps = append(steps,
		resource.TestStep{
			Config: testAccDeviceConfigFull(name + "-renamed"),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name+"-renamed")),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("ip"), knownvalue.StringExact("nas.example.com")),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("mac"), knownvalue.StringExact("AA-BB-CC-DD-EE-02")),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("wake_timeout"), knownvalue.Int64Exact(90)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("sol_port"), knownvalue.Int64Exact(8009)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("shutdown_cron"), knownvalue.StringExact("0 0 23 * * *")),
				statecheck.ExpectSensitiveValue(address, tfjsonpath.New("sol_password")),
				statecheck.ExpectSensitiveValue(address, tfjsonpath.New("wol_password")),
			},
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrPtr(address, "id", &id),
			),
		},
	)
	steps = append(steps, testAccImportSteps(address)...)
	steps = append(steps,
		resource.TestStep{
			Config: testAccDeviceConfigBasic(name),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("description"), knownvalue.StringExact("")),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("wake_cron_enabled"), knownvalue.Bool(false)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("wake_timeout"), knownvalue.Null()),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("sol_port"), knownvalue.Null()),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("netmask"), knownvalue.StringExact("255.255.255.255")),
			},
			Check: resource.TestCheckResourceAttrPtr(address, "id", &id),
		},
		resource.TestStep{
			PreConfig: func() {
				if _, err := testAccClient(t).Devices.Update(t.Context(), id, map[string]any{"name": "changed-out-of-band", "wake_timeout": 33}); err != nil {
					t.Fatal(err)
				}
			},
			Config:             testAccDeviceConfigBasic(name),
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			Config: testAccDeviceConfigBasic(name),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name)),
				statecheck.ExpectKnownValue(address, tfjsonpath.New("wake_timeout"), knownvalue.Null()),
			},
		},
	)
	steps = append(steps, testAccDriftDeletedSteps(testAccDeviceConfigBasic(name), func() {
		if err := testAccClient(t).Devices.Delete(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	})...)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

func TestAccDeviceResource_identity(t *testing.T) {
	testAccIdentityTest(t, "upsnap_device.test", testAccDeviceConfigBasic(testAccName("device")))
}

func TestAccDeviceResource_validation(t *testing.T) {
	cases := map[string]struct {
		attrs string
		err   string
	}{
		"mac":          {`mac = "aa:bb:cc"`, `is not a MAC address`},
		"ip":           {`ip = "localhost"`, `IPv4 address or a fully qualified host name`},
		"netmask":      {`netmask = "255.0.255.0"`, `not a contiguous IPv4 subnet mask`},
		"cron":         {`wake_cron = "@every 1m"`, `not a valid cron expression`},
		"cron enabled": {`wake_cron_enabled = true`, `wake_cron must be set when wake_cron_enabled is true`},
		"sol port":     {`sol_port = 70000`, `between 1 and 65535`},
		"wol password": {`wol_password = "abc"`, `wol_password`},
		"link open":    {`link_open = "popup"`, `value must be one of`},
		"group id":     {`group_ids = ["Not-An-ID"]`, `PocketBase record ID`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			attrs := tc.attrs
			for _, base := range []string{`ip = "192.0.2.1"`, `mac = "aa:bb:cc:dd:ee:ff"`} {
				if !strings.HasPrefix(attrs, strings.Fields(base)[0]+" ") {
					attrs += "\n  " + base
				}
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
provider "upsnap" {
  endpoint = "http://127.0.0.1:1"
  token    = "a.b.c"
}

resource "upsnap_device" "test" {
  name = "validation"
  %s
}
`, attrs),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile(tc.err),
				}},
			})
		})
	}
}
