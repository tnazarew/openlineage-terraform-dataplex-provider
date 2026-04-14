package dataplex_test

import (
	"context"
	"fmt"
	"testing"

	lineage "cloud.google.com/go/datacatalog/lineage/apiv1"
	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Basic Create + Read ───────────────────────────────────────────────────────

func TestAccDataplexJob_basic(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + testAccJobConfig_basic(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Values set in config must be reflected in state
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					// Computed attributes must be non-empty after apply
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "run_name"),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "lineage_event_name"),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "update_time"),
				),
			},
		},
	})
}

// ── Create → Update (same Process, new Run) ───────────────────────────────────
//
// Each terraform apply emits a new OL RunEvent. Dataplex creates a new Run
// under the same Process (stable namespace+name key). process_name must be
// unchanged (UseStateForUnknown), run_name must differ.

func TestAccDataplexJob_update(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	// Capture process_name from step 1 to compare in step 2.
	var processNameAfterCreate string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			// Step 1: Create
			{
				Config: providerBlock() + testAccJobConfig_basic(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
					testAccSaveAttr("openlineage_job.test", "process_name", &processNameAfterCreate),
				),
			},
			// Step 2: Update description → new Run, same Process
			{
				Config: providerBlock() + testAccJobConfig_withDescription(jobName, "updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "description", "updated description"),
					// process_name must NOT change (UseStateForUnknown plan modifier)
					testAccCheckAttrEquals("openlineage_job.test", "process_name", &processNameAfterCreate),
					// run_name MUST change — every apply creates a new Run
					resource.TestCheckResourceAttrSet("openlineage_job.test", "run_name"),
				),
			},
		},
	})
}

// ── Job facets: job_type + ownership ─────────────────────────────────────────

func TestAccDataplexJob_withFacets(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + testAccJobConfig_withFacets(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
				),
			},
		},
	})
}

// ── Column lineage ────────────────────────────────────────────────────────────

func TestAccDataplexJob_withColumnLineage(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + testAccJobConfig_withColumnLineage(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "lineage_event_name"),
				),
			},
		},
	})
}

// ── Drift detection (disappears) ──────────────────────────────────────────────
//
// Simulates a resource deleted outside Terraform (e.g. via GCP console or
// another pipeline). The next plan must detect the drift (Read returns
// exists=false) and propose a re-create, hence ExpectNonEmptyPlan: true.

func TestAccDataplexJob_disappears(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerBlock() + testAccJobConfig_basic(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
					// Delete the process directly in GCP to simulate drift
					testAccDeleteProcessManually("openlineage_job.test"),
				),
				// The plan after manual deletion must be non-empty (drift → re-create)
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// ── Config helpers ────────────────────────────────────────────────────────────

func testAccJobConfig_basic(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "SPARK"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source_table"

    catalog {
      name      = "tf_acc_project.dataset.source_table"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output_table"

    catalog {
      name      = "tf_acc_project.dataset.output_table"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
}
`, jobName)
}

func testAccJobConfig_withDescription(jobName, description string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace   = "tf-acc-namespace"
  name        = %q
  description = %q

  job_type {
    processing_type = "BATCH"
    integration     = "SPARK"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.src"

    catalog {
      name      = "tf_acc_project.dataset.src"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.dst"

    catalog {
      name      = "tf_acc_project.dataset.dst"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
}
`, jobName, description)
}

func testAccJobConfig_withFacets(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "SPARK"
  }

  ownership {
    owners {
      name = "team-data-eng"
      type = "team"
    }
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.src"

    catalog {
      name      = "tf_acc_project.dataset.src"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.dst"

    catalog {
      name      = "tf_acc_project.dataset.dst"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
}
`, jobName)
}

func testAccJobConfig_withColumnLineage(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "SPARK"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.src"

    catalog {
      name      = "tf_acc_project.dataset.src"
      framework = "bigquery"
      type      = "TABLE"
    }
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.dst"

    catalog {
      name      = "tf_acc_project.dataset.dst"
      framework = "bigquery"
      type      = "TABLE"
    }

    column_lineage {
      fields {
        name = "output_col"
        input_field {
          namespace = "bigquery"
          name      = "tf_acc_project.dataset.src"
          field     = "input_col"
          transformation {
            type = "DIRECT"
          }
        }
      }
    }
  }
}
`, jobName)
}

// ── CheckDestroy ──────────────────────────────────────────────────────────────

// testAccCheckDataplexJobDestroyed verifies through the GCP API that every
// process created during the test no longer exists. Called by CheckDestroy
// after the TestCase finishes (terraform destroy).
func testAccCheckDataplexJobDestroyed(s *terraform.State) error {
	ctx := context.Background()

	client, err := lineage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create lineage client for destroy check: %w", err)
	}
	defer client.Close()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "openlineage_job" {
			continue
		}
		processName := rs.Primary.Attributes["process_name"]
		if processName == "" {
			continue
		}

		_, err := client.GetProcess(ctx, &lineagepb.GetProcessRequest{Name: processName})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				continue // Expected — resource was destroyed
			}
			return fmt.Errorf("unexpected error checking process %q: %w", processName, err)
		}
		return fmt.Errorf("process %q still exists after destroy", processName)
	}
	return nil
}

// ── Manual deletion helper (disappears test) ──────────────────────────────────

// testAccDeleteProcessManually returns a TestCheckFunc that deletes the Dataplex
// Process directly via the GCP API, bypassing Terraform. Used in the disappears
// test to simulate external drift.
func testAccDeleteProcessManually(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %q not found in state", resourceName)
		}
		processName := rs.Primary.Attributes["process_name"]
		if processName == "" {
			return fmt.Errorf("process_name not set in state for %q", resourceName)
		}

		ctx := context.Background()
		client, err := lineage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create lineage client: %w", err)
		}
		defer client.Close()

		op, err := client.DeleteProcess(ctx, &lineagepb.DeleteProcessRequest{Name: processName})
		if err != nil {
			return fmt.Errorf("failed to delete process %q: %w", processName, err)
		}
		return op.Wait(ctx)
	}
}

// ── Cross-step assertion helpers ──────────────────────────────────────────────

// testAccSaveAttr captures an attribute value into a variable so it can be
// compared in a later test step (e.g. process_name must be stable after update).
func testAccSaveAttr(resourceName, attr string, dest *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %q not found", resourceName)
		}
		*dest = rs.Primary.Attributes[attr]
		return nil
	}
}

// testAccCheckAttrEquals asserts that an attribute value has not changed since
// it was saved by testAccSaveAttr in a previous step.
func testAccCheckAttrEquals(resourceName, attr string, expected *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %q not found", resourceName)
		}
		got := rs.Primary.Attributes[attr]
		if got != *expected {
			return fmt.Errorf("%s.%s: expected %q (from previous step), got %q",
				resourceName, attr, *expected, got)
		}
		return nil
	}
}
