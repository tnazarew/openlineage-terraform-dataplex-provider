/*
 * Copyright 2018-2026 contributors to the OpenLineage project
 * SPDX-License-Identifier: Apache-2.0
 */

package dataplex_test

// Acceptance tests that verify schema validation behaviour for the two facets
// that Dataplex enables: FacetJobType (→ job_type block) and FacetDatasetCatalog
// (→ catalog block inside inputs / outputs).
//
//   - Absent block           → plan succeeds  (null attribute value, validator skips)
//   - Empty-string attribute → plan succeeds  (empty strings intentionally allowed)
//   - Missing required attr  → plan fails     (objectvalidator.AlsoRequires fires)
//   - Complete block         → plan succeeds  (non-empty value, validator passes)
//
// Tests that expect an error stop at the config-validation phase and do NOT make
// any real Dataplex API calls. Tests that expect success perform a full
// create + destroy cycle against real GCP.

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// errInvalidAttrCombination matches the diagnostic summary produced by
// objectvalidator.AlsoRequires when a block is present but a required
// attribute/sub-block is absent entirely:
//
//	"Invalid Attribute Combination"
var errInvalidAttrCombination = regexp.MustCompile(`(?i)invalid attribute combination`)

// errTooFewItems matches the diagnostic produced by listvalidator.SizeAtLeast(1)
// when a required list block has no entries:
//
//	"List must contain at least 1 element(s)"
var errTooFewItems = regexp.MustCompile(`(?i)list must contain at least 1`)

// ── job_type block ────────────────────────────────────────────────────────────

// TestAccDataplexJob_noJobTypeBlock verifies that a config that omits the
// job_type block entirely is accepted — no validation error must be produced.
//
// Before the fix this test would fail because job_type had Required attributes
// (processing_type, integration) and the framework rejected any config that
// did not include them, even when the block was absent.
func TestAccDataplexJob_noJobTypeBlock(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// Config has NO job_type block — must plan and apply without error.
				Config: providerBlock() + testAccJobConfig_noJobType(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialJobTypeBlock_processingTypeEmpty verifies that
// setting processing_type = "" inside a job_type block is accepted (empty strings
// are intentionally allowed; AlsoRequires handles presence checks only).
func TestAccDataplexJob_partialJobTypeBlock_processingTypeEmpty(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// processing_type = "" must be accepted — empty strings are allowed.
				Config: providerBlock() + testAccJobConfig_jobTypeEmptyProcessingType(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialJobTypeBlock_integrationEmpty verifies that
// setting integration = "" inside a job_type block is accepted.
func TestAccDataplexJob_partialJobTypeBlock_integrationEmpty(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// integration = "" must be accepted — empty strings are allowed.
				Config: providerBlock() + testAccJobConfig_jobTypeEmptyIntegration(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// ── catalog block ─────────────────────────────────────────────────────────────

// TestAccDataplexJob_noCatalogBlock verifies that a config that omits the
// catalog block inside inputs / outputs is accepted — no validation error.
//
// Before the fix this test would fail because catalog.framework, catalog.type,
// and catalog.name were Required, forcing every dataset to carry a catalog block.
func TestAccDataplexJob_noCatalogBlock(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// Config has NO catalog blocks inside inputs / outputs.
				Config: providerBlock() + testAccJobConfig_noCatalog(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialCatalogBlock_frameworkEmpty verifies that setting
// catalog.framework = "" inside an inputs block is accepted (empty strings allowed).
func TestAccDataplexJob_partialCatalogBlock_frameworkEmpty(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// catalog.framework = "" must be accepted — empty strings are allowed.
				Config: providerBlock() + testAccJobConfig_catalogEmptyFramework(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialCatalogBlock_typeEmpty verifies that setting
// catalog.type = "" inside an inputs block is accepted.
func TestAccDataplexJob_partialCatalogBlock_typeEmpty(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// catalog.type = "" must be accepted — empty strings are allowed.
				Config: providerBlock() + testAccJobConfig_catalogEmptyType(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialCatalogBlock_nameEmpty verifies that setting
// catalog.name = "" inside an inputs block is accepted.
func TestAccDataplexJob_partialCatalogBlock_nameEmpty(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		CheckDestroy:             testAccCheckDataplexJobDestroyed,
		Steps: []resource.TestStep{
			{
				// catalog.name = "" must be accepted — empty strings are allowed.
				Config: providerBlock() + testAccJobConfig_catalogEmptyName(jobName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("openlineage_job.test", "namespace", "tf-acc-namespace"),
					resource.TestCheckResourceAttr("openlineage_job.test", "name", jobName),
					resource.TestCheckResourceAttrSet("openlineage_job.test", "process_name"),
				),
			},
		},
	})
}

// TestAccDataplexJob_partialCatalogBlock_frameworkAbsent verifies that omitting
// catalog.framework entirely (attribute not defined at all, not just empty) also
// fails validation. This is distinct from the empty-string case: the attribute
// value is null rather than "", but objectvalidator.AlsoRequires on the catalog
// block catches this and rejects the config.
//
// Config example that must fail:
//
//	inputs {
//	  namespace = "bigquery"
//	  name      = "project.dataset.src"
//	  catalog {
//	    type = "TABLE"
//	    name = "project.dataset.src"
//	    # framework is absent — must fail
//	  }
//	}
func TestAccDataplexJob_partialCatalogBlock_frameworkAbsent(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				// catalog block is present but framework is not set at all.
				// objectvalidator.AlsoRequires must fire: "Invalid Attribute Combination".
				Config:      providerBlock() + testAccJobConfig_catalogNoFramework(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// ── config helpers ────────────────────────────────────────────────────────────

// testAccJobConfig_noJobType is a minimal valid config that intentionally
// omits the job_type block. Dataplex enables FacetJobType, so before Option A
// this config would have been rejected with "Missing required argument".
func testAccJobConfig_noJobType(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  # job_type block intentionally absent — must not produce a validation error

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      name      = "tf_acc_project.dataset.source"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      name      = "tf_acc_project.dataset.output"
      framework = "bigquery"
      type      = "TABLE"
    }
  }
}
`, jobName)
}

// testAccJobConfig_noCatalog is a valid config that omits all catalog blocks
// inside inputs and outputs. Before Option A this would have been rejected with
// "Missing required argument" for catalog.framework / .type / .name.
func testAccJobConfig_noCatalog(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"
    # catalog block intentionally absent
  }
  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"
    # catalog block intentionally absent
  }
}
`, jobName)
}

// testAccJobConfig_jobTypeEmptyProcessingType sets processing_type = "" to
// trigger the LengthAtLeast(1) validator on that attribute.
func testAccJobConfig_jobTypeEmptyProcessingType(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = ""      # must fail: LengthAtLeast(1)
    integration     = "BYOL"
  }
}
`, jobName)
}

// testAccJobConfig_jobTypeEmptyIntegration sets integration = "" to trigger
// the LengthAtLeast(1) validator on that attribute.
func testAccJobConfig_jobTypeEmptyIntegration(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = ""      # must fail: LengthAtLeast(1)
  }
}
`, jobName)
}

// testAccJobConfig_catalogEmptyFramework sets catalog.framework = "" to trigger
// the LengthAtLeast(1) validator inside the catalog block.
func testAccJobConfig_catalogEmptyFramework(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = ""                              # must fail: LengthAtLeast(1)
      type      = "TABLE"
      name      = "tf_acc_project.dataset.source"
    }
  }
}
`, jobName)
}

// testAccJobConfig_catalogEmptyType sets catalog.type = "" to trigger the
// LengthAtLeast(1) validator inside the catalog block.
func testAccJobConfig_catalogEmptyType(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = "bigquery"
      type      = ""                              # must fail: LengthAtLeast(1)
      name      = "tf_acc_project.dataset.source"
    }
  }
}
`, jobName)
}

// testAccJobConfig_catalogEmptyName sets catalog.name = "" to trigger the
// LengthAtLeast(1) validator inside the catalog block.
func testAccJobConfig_catalogEmptyName(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = ""                              # must fail: LengthAtLeast(1)
    }
  }
}
`, jobName)
}

// testAccJobConfig_catalogNoFramework produces a catalog block that has type and
// name set but completely omits framework. This exercises the
// objectvalidator.AlsoRequires constraint: when the block is present, all three
// key attributes must be provided.
func testAccJobConfig_catalogNoFramework(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      # framework intentionally absent (not set at all, not just empty)
      type = "TABLE"
      name = "tf_acc_project.dataset.source"
    }
  }
}
`, jobName)
}

// ── column_lineage block ──────────────────────────────────────────────────────

// TestAccDataplexJob_columnLineage_noFieldsBlock verifies that a column_lineage
// block with zero fields entries is rejected. When no fields {} blocks are
// written the list is null — objectvalidator.AlsoRequires on columnLineageBlock
// fires: "Invalid Attribute Combination".
func TestAccDataplexJob_columnLineage_noFieldsBlock(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageNoFields(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_columnLineage_fieldMissingName verifies that a fields entry
// without a name attribute is rejected (objectvalidator.AlsoRequires on NestedObject).
func TestAccDataplexJob_columnLineage_fieldMissingName(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageFieldMissingName(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_columnLineage_fieldNoInputFields verifies that a fields entry
// with no input_field blocks is rejected. The missing input_field list is null —
// AlsoRequires on the fields NestedObject fires: "Invalid Attribute Combination".
func TestAccDataplexJob_columnLineage_fieldNoInputFields(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageFieldNoInputFields(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_columnLineage_inputFieldMissingNamespace verifies that an
// input_field item without namespace is rejected (AlsoRequires on NestedObject).
func TestAccDataplexJob_columnLineage_inputFieldMissingNamespace(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageInputFieldMissingNamespace(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_columnLineage_inputFieldMissingTransformation verifies that
// an input_field without a transformation block is rejected.
func TestAccDataplexJob_columnLineage_inputFieldMissingTransformation(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageInputFieldMissingTransformation(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_columnLineage_transformationMissingType verifies that a
// transformation block without type is rejected (AlsoRequires("type") on the block).
func TestAccDataplexJob_columnLineage_transformationMissingType(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_columnLineageTransformationMissingType(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// ── column_lineage config helpers ─────────────────────────────────────────────

// baseJobWithOutput returns a job resource with one output dataset (no column_lineage),
// used as the base for column_lineage test configs.
func baseJobOutputBlock(namespace, name string) string {
	return fmt.Sprintf(`
  outputs {
    namespace = %q
    name      = %q

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = %q
    }
`, namespace, name, name)
}

func testAccJobConfig_columnLineageNoFields(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      # fields block intentionally absent — must fail: SizeAtLeast(1)
    }
  }
}
`, jobName)
}

func testAccJobConfig_columnLineageFieldMissingName(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      fields {
        # name intentionally absent — must fail: AlsoRequires("name")

        input_field {
          namespace = "bigquery"
          name      = "tf_acc_project.dataset.input"
          field     = "col"
          transformation { type = "DIRECT" }
        }
      }
    }
  }
}
`, jobName)
}

func testAccJobConfig_columnLineageFieldNoInputFields(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      fields {
        name = "output_col"
        # input_field intentionally absent — must fail: SizeAtLeast(1)
      }
    }
  }
}
`, jobName)
}

func testAccJobConfig_columnLineageInputFieldMissingNamespace(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      fields {
        name = "output_col"

        input_field {
          # namespace intentionally absent — must fail: AlsoRequires
          name  = "tf_acc_project.dataset.input"
          field = "col"
          transformation { type = "DIRECT" }
        }
      }
    }
  }
}
`, jobName)
}

func testAccJobConfig_columnLineageInputFieldMissingTransformation(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      fields {
        name = "output_col"

        input_field {
          namespace = "bigquery"
          name      = "tf_acc_project.dataset.input"
          field     = "col"
          # transformation block intentionally absent — must fail: AlsoRequires
        }
      }
    }
  }
}
`, jobName)
}

func testAccJobConfig_columnLineageTransformationMissingType(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  outputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.output"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.output"
    }

    column_lineage {
      fields {
        name = "output_col"

        input_field {
          namespace = "bigquery"
          name      = "tf_acc_project.dataset.input"
          field     = "col"

          transformation {
            # type intentionally absent — must fail: AlsoRequires("type")
            subtype = "IDENTITY"
          }
        }
      }
    }
  }
}
`, jobName)
}

// ── symlinks block ────────────────────────────────────────────────────────────
//
// FacetDatasetSymlinks is enabled in the Dataplex capability, so the full
// symlinksBlock() with its NestedObject.Validators is active.

// TestAccDataplexJob_symlinks_missingNamespace verifies that a symlinks entry
// with namespace absent is rejected (AlsoRequires on the NestedObject).
func TestAccDataplexJob_symlinks_missingNamespace(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				// namespace intentionally absent — must fail.
				Config:      providerBlock() + testAccJobConfig_symlinksMissingNamespace(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_symlinks_missingName verifies that a symlinks entry with
// name absent is rejected.
func TestAccDataplexJob_symlinks_missingName(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_symlinksMissingName(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// TestAccDataplexJob_symlinks_missingType verifies that a symlinks entry with
// type absent is rejected.
func TestAccDataplexJob_symlinks_missingType(t *testing.T) {
	jobName := "tf-acc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      providerBlock() + testAccJobConfig_symlinksMissingType(jobName),
				ExpectError: errInvalidAttrCombination,
			},
		},
	})
}

// ── symlinks config helpers ───────────────────────────────────────────────────

func testAccJobConfig_symlinksMissingNamespace(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.source"
    }

    symlinks {
      # namespace intentionally absent
      name = "alternate_name"
      type = "TABLE"
    }
  }
}
`, jobName)
}

func testAccJobConfig_symlinksMissingName(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.source"
    }

    symlinks {
      namespace = "alternate_namespace"
      # name intentionally absent
      type = "TABLE"
    }
  }
}
`, jobName)
}

func testAccJobConfig_symlinksMissingType(jobName string) string {
	return fmt.Sprintf(`
resource "openlineage_job" "test" {
  namespace = "tf-acc-namespace"
  name      = %q

  job_type {
    processing_type = "BATCH"
    integration     = "BYOL"
  }

  inputs {
    namespace = "bigquery"
    name      = "tf_acc_project.dataset.source"

    catalog {
      framework = "bigquery"
      type      = "TABLE"
      name      = "tf_acc_project.dataset.source"
    }

    symlinks {
      namespace = "alternate_namespace"
      name      = "alternate_name"
      # type intentionally absent
    }
  }
}
`, jobName)
}


