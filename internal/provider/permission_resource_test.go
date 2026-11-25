package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const testAccPermissionUserPassword = "permission-password"

func testAccPermissionConfig(name, permission string) string {
	return fmt.Sprintf(`
resource "upsnap_user" "test" {
  username = %[1]q
  password = %[2]q
}

resource "upsnap_device" "a" {
  name = "%[1]s-a"
  ip   = "192.0.2.51"
  mac  = "aa:bb:cc:dd:ee:51"
}

resource "upsnap_device" "b" {
  name = "%[1]s-b"
  ip   = "192.0.2.52"
  mac  = "aa:bb:cc:dd:ee:52"
}

resource "upsnap_permission" "test" {
  user_id = upsnap_user.test.id
  %[3]s
}
`, name, testAccPermissionUserPassword, permission)
}

// testAccCheckVisibleToUser logs in as the managed user, the way the provider does with
// auth_collection = "users", and checks which test devices the permission lets it see.
// A second provider alias cannot be used for this: reattached test providers share one server.
func testAccCheckVisibleToUser(name string, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		c, err := client.New(client.Config{
			Endpoint:       os.Getenv("UPSNAP_ENDPOINT"),
			Username:       name,
			Password:       testAccPermissionUserPassword,
			AuthCollection: client.UsersCollection,
		})
		if err != nil {
			return err
		}
		devices, err := c.Devices.List(context.Background(), client.ListOptions{Filter: `name ~ "` + name + `"`, Sort: "name"})
		if err != nil {
			return err
		}
		got := make([]string, len(devices))
		for i, d := range devices {
			got[i] = strings.TrimPrefix(d.Name, name+"-")
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("user %s sees devices %v, want %v", name, got, want)
		}
		return nil
	}
}

func TestAccPermissionResource(t *testing.T) {
	name := testAccName("perm")
	address := "upsnap_permission.test"
	var id, userID string

	initial := testAccPermissionConfig(name, `
  read  = [upsnap_device.a.id]
  power = [upsnap_device.a.id]
`)
	updated := testAccPermissionConfig(name, `
  create = true
  read   = [upsnap_device.a.id, upsnap_device.b.id]
  update = [upsnap_device.b.id]
  delete = [upsnap_device.b.id]
  power  = [upsnap_device.a.id, upsnap_device.b.id]
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initial,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(address, tfjsonpath.New("user_id"), "upsnap_user.test", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("create"), knownvalue.Bool(false)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("read"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("update"), knownvalue.SetSizeExact(0)),
				},
				Check: resource.ComposeTestCheckFunc(
					testAccCaptureID(address, &id),
					testAccCaptureID("upsnap_user.test", &userID),
					resource.TestCheckTypeSetElemAttrPair(address, "power.*", "upsnap_device.a", "id"),
					testAccCheckVisibleToUser(name, "a"),
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName: address,
				ImportState:  true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return "user:" + userID, nil
				},
				ImportStateVerify: true,
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateId:     "username:" + name,
				ImportStateVerify: true,
			},
			{
				ResourceName:    address,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
			},
			{
				Config: updated,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("create"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("read"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("delete"), knownvalue.SetSizeExact(1)),
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPtr(address, "id", &id),
					testAccCheckVisibleToUser(name, "a", "b"),
				),
			},
			{
				Config: updated + `
resource "upsnap_permission" "duplicate" {
  user_id = upsnap_user.test.id
}
`,
				ExpectError: regexp.MustCompile(`Permissions already exist for this user`),
			},
			{
				PreConfig: func() {
					if err := testAccClient(t).Permissions.Delete(t.Context(), id); err != nil {
						t.Fatal(err)
					}
				},
				Config:             updated,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: updated,
			},
		},
	})
}

func TestAccPermissionResource_identity(t *testing.T) {
	testAccIdentityTest(t, "upsnap_permission.test", testAccPermissionConfig(testAccName("perm"), ""))
}

func TestAccPermissionResource_importErrors(t *testing.T) {
	name := testAccName("perm")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: testAccPermissionConfig(name, "")},
			{
				ResourceName:  "upsnap_permission.test",
				ImportState:   true,
				ImportStateId: "username:no-such-user",
				ExpectError:   regexp.MustCompile(`No record has username "no-such-user"`),
			},
			{
				ResourceName:  "upsnap_permission.test",
				ImportState:   true,
				ImportStateId: "email:someone@example.com",
				ExpectError:   regexp.MustCompile(`Unknown import ID prefix "email"`),
			},
		},
	})
}
