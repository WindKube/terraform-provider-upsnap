package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccUserConfig(username, extra string) string {
	return fmt.Sprintf(`
resource "upsnap_user" "test" {
  username = %q
  %s
}
`, username, extra)
}

func testAccCheckUserLogin(username, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := client.New(client.Config{
			Endpoint:       os.Getenv("UPSNAP_ENDPOINT"),
			Username:       username,
			Password:       password,
			AuthCollection: client.UsersCollection,
		})
		if err != nil {
			return err
		}
		return c.Authenticate(context.Background())
	}
}

func TestAccUserResource(t *testing.T) {
	username := testAccName("user")
	address := "upsnap_user.test"
	var id string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig(username, `password = "first-password"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("username"), knownvalue.StringExact(username)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("email"), knownvalue.StringExact("")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("avatar"), knownvalue.Int64Exact(0)),
					statecheck.ExpectSensitiveValue(address, tfjsonpath.New("password")),
				},
				Check: resource.ComposeTestCheckFunc(
					testAccCaptureID(address, &id),
					testAccCheckUserLogin(username, "first-password"),
				),
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateId:           "username:" + username,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:       address,
				ImportState:        true,
				ImportStateKind:    resource.ImportBlockWithID,
				ExpectNonEmptyPlan: true,
				ImportPlanChecks: resource.ImportPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate),
				}},
			},
			{
				Config: testAccUserConfig(username, `
  password         = "second-password"
  email            = "tfacc@example.com"
  email_visibility = true
  verified         = true
  avatar           = 7
`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("email"), knownvalue.StringExact("tfacc@example.com")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("verified"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("avatar"), knownvalue.Int64Exact(7)),
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPtr(address, "id", &id),
					testAccCheckUserLogin(username, "second-password"),
				),
			},
			{
				Config: testAccUserConfig(username+"-renamed", `password = "second-password"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("username"), knownvalue.StringExact(username+"-renamed")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("email"), knownvalue.StringExact("")),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("avatar"), knownvalue.Int64Exact(0)),
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPtr(address, "id", &id),
					testAccCheckUserLogin(username+"-renamed", "second-password"),
				),
			},
			{
				PreConfig: func() {
					if err := testAccClient(t).Users.Delete(t.Context(), id); err != nil {
						t.Fatal(err)
					}
				},
				Config:             testAccUserConfig(username+"-renamed", `password = "second-password"`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccUserConfig(username+"-renamed", `password = "second-password"`),
			},
		},
	})
}

func TestAccUserResource_identity(t *testing.T) {
	testAccIdentityTestWithImportPlan(t, "upsnap_user.test", testAccUserConfig(testAccName("user"), `password = "identity-password"`),
		plancheck.ExpectResourceAction("upsnap_user.test", plancheck.ResourceActionUpdate))
}
