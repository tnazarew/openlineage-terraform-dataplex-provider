package dataplex_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/tnazarew/terraform-provider-openlineage-dataplex/internal/dataplex"
)

// testAccProviderFactories wires every TestAcc* test to the local provider
// implementation (not the registry). "test" is used as the version string.
var testAccProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openlineage": providerserver.NewProtocol6WithError(dataplex.New("test")()),
}

// testAccPreCheck validates that the required environment variables are set
// before each acceptance test runs. resource.Test() calls PreCheck first, so
// a missing variable produces a clear Fatal instead of an opaque network error.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if v := os.Getenv("GCP_PROJECT_ID"); v == "" {
		t.Fatal("GCP_PROJECT_ID must be set for acceptance tests")
	}
	if v := os.Getenv("GCP_REGION"); v == "" {
		t.Fatal("GCP_REGION must be set for acceptance tests")
	}
}

// providerBlock returns a provider{} configuration block populated from
// environment variables. Used as a prefix in every acceptance test config.
func providerBlock() string {
	return fmt.Sprintf(`
provider "openlineage" {
  project_id = %q
  region     = %q
}
`, os.Getenv("GCP_PROJECT_ID"), os.Getenv("GCP_REGION"))
}

// TestAccProvider_configure verifies that the provider configures itself
// without errors when valid credentials are available. No resource is
// needed — this tests Configure() in isolation.
func TestAccProvider_configure(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				// A minimal valid config — just the provider block, no resources.
				// Passes only if Configure() returns no errors.
				Config: providerBlock(),
			},
		},
	})
}

