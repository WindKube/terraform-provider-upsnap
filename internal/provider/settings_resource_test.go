package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

var clientListOne = client.ListOptions{PerPage: 1}

// testAccRestoreSettings snapshots UpSnap's settings and restores them when the test ends.
func testAccRestoreSettings(t *testing.T) {
	t.Helper()
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("%s not set", resource.EnvTfAcc)
	}
	c := testAccClient(t)
	priv, err := c.SettingsPrivate.List(t.Context(), clientListOne)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := c.SettingsPublic.List(t.Context(), clientListOne)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := c.SettingsPrivate.Update(ctx, priv[0].ID, priv[0]); err != nil {
			t.Errorf("restoring private settings: %v", err)
		}
		if _, err := c.SettingsPublic.Update(ctx, pub[0].ID, map[string]any{"website_title": pub[0].WebsiteTitle}); err != nil {
			t.Errorf("restoring public settings: %v", err)
		}
	})
}

func testAccSettingsConfig(attrs string) string {
	return fmt.Sprintf(`
resource "upsnap_settings" "this" {
  %s
}
`, attrs)
}

func TestAccSettingsResource(t *testing.T) {
	testAccRestoreSettings(t)
	address := "upsnap_settings.this"
	full := testAccSettingsConfig(`
  interval          = "*/10 * * * * *"
  scan_range        = "192.168.77.0/24"
  lazy_ping         = true
  track_ip_interval = "0 */5 * * * *"
  website_title     = "TF Acc UpSnap"
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSettingsSurviveDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: full,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("id"), knownvalue.StringExact("settings")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("interval"), knownvalue.StringExact("*/10 * * * * *")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("scan_range"), knownvalue.StringExact("192.168.77.0/24")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("lazy_ping"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("website_title"), knownvalue.StringExact("TF Acc UpSnap")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("setup_completed"), knownvalue.Bool(true)),
				},
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "settings",
				ImportStateVerify: true,
			},
			{
				ResourceName:    address,
				ImportState:     true,
				ImportStateId:   "settings",
				ImportStateKind: resource.ImportBlockWithID,
			},
			{
				Config: testAccSettingsConfig(`
  interval      = "*/5 * * * * *"
  lazy_ping     = false
  website_title = "TF Acc UpSnap 2"
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("interval"), knownvalue.StringExact("*/5 * * * * *")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("lazy_ping"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("website_title"), knownvalue.StringExact("TF Acc UpSnap 2")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("scan_range"), knownvalue.StringExact("192.168.77.0/24")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("track_ip_interval"), knownvalue.StringExact("0 */5 * * * *")),
				},
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					pub, err := c.SettingsPublic.List(t.Context(), clientListOne)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := c.SettingsPublic.Update(t.Context(), pub[0].ID, map[string]any{"website_title": "changed out of band"}); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccSettingsConfig(`
  interval      = "*/5 * * * * *"
  lazy_ping     = false
  website_title = "TF Acc UpSnap 2"
`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSettingsConfig(`
  interval      = "*/5 * * * * *"
  lazy_ping     = false
  website_title = "TF Acc UpSnap 2"
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("website_title"), knownvalue.StringExact("TF Acc UpSnap 2")),
				},
			},
		},
	})
}

// testAccCheckSettingsSurviveDestroy checks that destroying the resource left UpSnap's settings alone.
func testAccCheckSettingsSurviveDestroy(t *testing.T) resource.TestCheckFunc {
	return func(*terraform.State) error {
		pub, err := testAccClient(t).SettingsPublic.List(context.Background(), clientListOne)
		if err != nil {
			return err
		}
		if len(pub) != 1 || pub[0].WebsiteTitle != "TF Acc UpSnap 2" {
			return fmt.Errorf("settings changed on destroy: %+v", pub)
		}
		return nil
	}
}

func TestAccSettingsResource_identity(t *testing.T) {
	testAccRestoreSettings(t)
	testAccIdentityTest(t, "upsnap_settings.this", testAccSettingsConfig(`website_title = "TF Acc identity"`))
}

func TestAccSettingsResource_badImportID(t *testing.T) {
	testAccRestoreSettings(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testAccSettingsConfig("")},
			{
				ResourceName:  "upsnap_settings.this",
				ImportState:   true,
				ImportStateId: "something-else",
				ExpectError:   regexp.MustCompile(`singleton`),
			},
		},
	})
}
