package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, env := range []string{"UPSNAP_ENDPOINT", "UPSNAP_USERNAME", "UPSNAP_PASSWORD"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests (run `make testacc-up`)", env)
		}
	}
}

// testAccClient talks to UpSnap directly, to arrange and verify state out of band.
func testAccClient(t *testing.T) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		Endpoint: os.Getenv("UPSNAP_ENDPOINT"),
		Username: os.Getenv("UPSNAP_USERNAME"),
		Password: os.Getenv("UPSNAP_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testAccIsOpenTofu() bool {
	return strings.Contains(filepath.Base(os.Getenv("TF_ACC_TERRAFORM_PATH")), "tofu")
}

// testAccSkipOpenTofu skips features OpenTofu does not implement, such as resource identity and actions.
func testAccSkipOpenTofu(t *testing.T) {
	t.Helper()
	if testAccIsOpenTofu() {
		t.Skip("not supported by OpenTofu")
	}
}

func testAccName(prefix string) string {
	return "tfacc-" + prefix + "-" + acctest.RandString(8)
}

func testAccCaptureID(address string, id *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("%s not found in state", address)
		}
		*id = rs.Primary.ID
		if *id == "" {
			return fmt.Errorf("%s has no id", address)
		}
		return nil
	}
}

// testAccImportSteps verifies import with the CLI and with an import block by ID.
func testAccImportSteps(address string) []resource.TestStep {
	return []resource.TestStep{
		{
			ResourceName:      address,
			ImportState:       true,
			ImportStateVerify: true,
		},
		{
			ResourceName:    address,
			ImportState:     true,
			ImportStateKind: resource.ImportBlockWithID,
		},
	}
}

// testAccIdentityTest checks the identity a resource records and imports it with an import
// block that sets identity instead of id (Terraform 1.12+).
func testAccIdentityTest(t *testing.T, address, config string) {
	t.Helper()
	testAccIdentityTestWithImportPlan(t, address, config)
}

// testAccIdentityTestWithImportPlan is testAccIdentityTest for resources whose import plans a
// change, such as a user whose password cannot be read back; checks assert what that change is.
func testAccIdentityTestWithImportPlan(t *testing.T, address, config string, checks ...plancheck.PlanCheck) {
	t.Helper()
	testAccSkipOpenTofu(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_12_0)},
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectIdentity(address, map[string]knownvalue.Check{"id": knownvalue.NotNull()}),
					statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")),
				},
			},
			{
				ResourceName:       address,
				ImportState:        true,
				ImportStateKind:    resource.ImportBlockWithResourceIdentity,
				ExpectNonEmptyPlan: len(checks) > 0,
				ImportPlanChecks:   resource.ImportPlanChecks{PreApply: checks},
			},
		},
	})
}

// testAccDriftDeletedSteps deletes the resource behind Terraform's back, expects the refresh
// to notice, and expects the next apply to recreate it.
func testAccDriftDeletedSteps(config string, deleteOutOfBand func()) []resource.TestStep {
	return []resource.TestStep{
		{
			PreConfig:          deleteOutOfBand,
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		{
			Config: config,
		},
	}
}
