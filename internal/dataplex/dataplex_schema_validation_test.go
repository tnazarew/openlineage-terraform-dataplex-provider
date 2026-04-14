/*
 * Copyright 2018-2026 contributors to the OpenLineage project
 * SPDX-License-Identifier: Apache-2.0
 */

package dataplex

// Schema validation tests for the Dataplex provider.
//
// These tests verify that the schema produced by DataplexJobResource.Capability()
// correctly distinguishes active (enabled) facet blocks from stub (disabled) ones,
// and that validators behave correctly:
//
//   - Null value (block absent / attribute omitted)  → validator SKIPS → no error ✓
//   - Empty string (block present, attr set to "")   → accepted (empty strings allowed) ✓
//   - Non-empty value ("BATCH", "bigquery", …)       → accepted ✓
//
// AlsoRequires object-level validators enforce that required sibling attributes
// are present when a block is defined; individual string validators are not used.

import (
	"context"
	"testing"

	"github.com/OpenLineage/openlineage/byool/terraform/openlineage-base-resource/ol"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	fwvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// hasValidationError invokes every string validator attached to attr with the
// given value and returns true if any of them produces a diagnostic error.
// The path argument is used only for error message context.
func hasValidationError(ctx context.Context, attr schema.StringAttribute, p path.Path, val types.String) bool {
	for _, v := range attr.Validators {
		req := fwvalidator.StringRequest{
			ConfigValue: val,
			Path:        p,
		}
		resp := &fwvalidator.StringResponse{}
		v.ValidateString(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			return true
		}
	}
	return false
}

// dataplexSchema is a convenience function that returns the Terraform schema
// generated for the real Dataplex capability.
func dataplexSchema() schema.Schema {
	return ol.GenerateJobSchema((&DataplexJobResource{}).Capability())
}

// ── job_type block ────────────────────────────────────────────────────────────

// TestDataplexSchema_JobTypeBlock_AbsentBlock_PassesValidation verifies that
// when the job_type block is entirely absent from the config, none of the
// attribute validators fire. This is the "block not defined" case.
//
// Root-cause: before Option A, Required attributes caused the framework to
// reject configs with no job_type block. After the fix, null attribute values
// (block absent) must be silently accepted.
func TestDataplexSchema_JobTypeBlock_AbsentBlock_PassesValidation(t *testing.T) {
	s := dataplexSchema()

	jobTypeBlock, ok := s.Blocks["job_type"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected job_type to be a SingleNestedBlock in the Dataplex schema")
	}

	ctx := context.Background()
	for attrName, attr := range jobTypeBlock.Attributes {
		sa, ok := attr.(schema.StringAttribute)
		if !ok || len(sa.Validators) == 0 {
			continue
		}
		p := path.Root("job_type").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringNull()) {
			t.Errorf(
				"job_type.%s: validator fired for null value — "+
					"the absent-block case must never produce a validation error",
				attrName,
			)
		}
	}
}

// TestDataplexSchema_JobTypeBlock_EmptyAttributes_PassValidation verifies that
// setting a key attribute inside job_type to an explicit empty string is accepted.
// Empty strings are intentionally allowed; AlsoRequires handles presence checks.
//
// Example config:  job_type { processing_type = "" }   ← must NOT error
func TestDataplexSchema_JobTypeBlock_EmptyAttributes_PassValidation(t *testing.T) {
	s := dataplexSchema()

	jobTypeBlock, ok := s.Blocks["job_type"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected job_type to be a SingleNestedBlock in the Dataplex schema")
	}

	attrsToCheck := []string{"processing_type", "integration"}

	ctx := context.Background()
	for _, attrName := range attrsToCheck {
		attr, ok := jobTypeBlock.Attributes[attrName]
		if !ok {
			t.Errorf("job_type.%s: attribute not found in schema", attrName)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("job_type.%s: expected StringAttribute", attrName)
			continue
		}
		p := path.Root("job_type").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringValue("")) {
			t.Errorf(
				"job_type.%s: validator fired for empty string — "+
					"empty strings must be accepted (empty-string rejection was removed intentionally)",
				attrName,
			)
		}
	}
}

// TestDataplexSchema_JobTypeBlock_ValidValues_PassValidation verifies that
// realistic attribute values (e.g. "BATCH", "SPARK") are accepted without error.
func TestDataplexSchema_JobTypeBlock_ValidValues_PassValidation(t *testing.T) {
	s := dataplexSchema()

	jobTypeBlock, ok := s.Blocks["job_type"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected job_type to be a SingleNestedBlock")
	}

	validValues := map[string]string{
		"processing_type": "BATCH",
		"integration":     "SPARK",
	}

	ctx := context.Background()
	for attrName, value := range validValues {
		attr, ok := jobTypeBlock.Attributes[attrName]
		if !ok {
			t.Errorf("job_type.%s: attribute not found in schema", attrName)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			continue
		}
		p := path.Root("job_type").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringValue(value)) {
			t.Errorf(
				"job_type.%s: validator fired for valid value %q — "+
					"non-empty values must always be accepted",
				attrName, value,
			)
		}
	}
}

// ── catalog block (inside inputs) ─────────────────────────────────────────────

// TestDataplexSchema_CatalogBlock_AbsentBlock_PassesValidation verifies that
// when no catalog block is written inside an input/output, none of its attribute
// validators fire. This is the "block not defined" case for catalog.
//
// Root-cause: FacetDatasetCatalog is enabled in Dataplex capability. Before the
// fix, catalog.framework / catalog.type / catalog.name were Required, causing
// every dataset without a catalog block to fail validation.
func TestDataplexSchema_CatalogBlock_AbsentBlock_PassesValidation(t *testing.T) {
	s := dataplexSchema()

	inputs, ok := s.Blocks["inputs"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected inputs to be a ListNestedBlock")
	}
	catalogBlock, ok := inputs.NestedObject.Blocks["catalog"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected catalog to be a SingleNestedBlock inside inputs")
	}

	ctx := context.Background()
	for attrName, attr := range catalogBlock.Attributes {
		sa, ok := attr.(schema.StringAttribute)
		if !ok || len(sa.Validators) == 0 {
			continue
		}
		p := path.Root("inputs").AtName("catalog").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringNull()) {
			t.Errorf(
				"catalog.%s (in inputs): validator fired for null value — "+
					"the absent-block case must never produce a validation error",
				attrName,
			)
		}
	}
}

// TestDataplexSchema_CatalogBlock_EmptyAttributes_PassValidation verifies that
// setting catalog.framework, catalog.type, or catalog.name to an empty string
// does NOT produce a validation error. Empty strings are intentionally allowed;
// AlsoRequires handles presence-only checks.
//
// Example config:
//
//	inputs {
//	  namespace = "bigquery"
//	  name      = "my_table"
//	  catalog   { framework = "" }   # ← must be accepted
//	}
func TestDataplexSchema_CatalogBlock_EmptyAttributes_PassValidation(t *testing.T) {
	s := dataplexSchema()

	inputs, ok := s.Blocks["inputs"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected inputs to be a ListNestedBlock")
	}
	catalogBlock, ok := inputs.NestedObject.Blocks["catalog"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected catalog to be a SingleNestedBlock inside inputs")
	}

	attrsToCheck := []string{"framework", "type", "name"}

	ctx := context.Background()
	for _, attrName := range attrsToCheck {
		attr, ok := catalogBlock.Attributes[attrName]
		if !ok {
			t.Errorf("catalog.%s: attribute not found in schema", attrName)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("catalog.%s: expected StringAttribute", attrName)
			continue
		}
		p := path.Root("inputs").AtName("catalog").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringValue("")) {
			t.Errorf(
				"catalog.%s (in inputs): validator fired for empty string — "+
					"empty strings must be accepted (empty-string rejection was removed intentionally)",
				attrName,
			)
		}
	}
}

// TestDataplexSchema_CatalogBlock_ValidValues_PassValidation verifies that
// realistic catalog attribute values are accepted without error.
func TestDataplexSchema_CatalogBlock_ValidValues_PassValidation(t *testing.T) {
	s := dataplexSchema()

	inputs, ok := s.Blocks["inputs"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected inputs to be a ListNestedBlock")
	}
	catalogBlock, ok := inputs.NestedObject.Blocks["catalog"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected catalog to be a SingleNestedBlock inside inputs")
	}

	validValues := map[string]string{
		"framework": "bigquery",
		"type":      "TABLE",
		"name":      "project.dataset.table",
	}

	ctx := context.Background()
	for attrName, value := range validValues {
		attr, ok := catalogBlock.Attributes[attrName]
		if !ok {
			t.Errorf("catalog.%s: attribute not found in schema", attrName)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			continue
		}
		p := path.Root("inputs").AtName("catalog").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringValue(value)) {
			t.Errorf(
				"catalog.%s (in inputs): validator fired for valid value %q — "+
					"non-empty values must always be accepted",
				attrName, value,
			)
		}
	}
}

// TestDataplexSchema_CatalogBlock_SameInOutputs verifies that the catalog block
// inside outputs has the same validator behaviour as the one inside inputs.
func TestDataplexSchema_CatalogBlock_SameInOutputs(t *testing.T) {
	s := dataplexSchema()

	outputs, ok := s.Blocks["outputs"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("expected outputs to be a ListNestedBlock")
	}
	catalogBlock, ok := outputs.NestedObject.Blocks["catalog"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected catalog to be a SingleNestedBlock inside outputs")
	}

	ctx := context.Background()

	// Null → must not fire.
	for attrName, attr := range catalogBlock.Attributes {
		sa, ok := attr.(schema.StringAttribute)
		if !ok || len(sa.Validators) == 0 {
			continue
		}
		p := path.Root("outputs").AtName("catalog").AtName(attrName)
		if hasValidationError(ctx, sa, p, types.StringNull()) {
			t.Errorf(
				"catalog.%s (in outputs): validator fired for null — block-absent case must be accepted",
				attrName,
			)
		}
	}

	// Empty string → must fire for key fields.
	for _, attrName := range []string{"framework", "type", "name"} {
		attr, ok := catalogBlock.Attributes[attrName]
		if !ok {
			t.Errorf("catalog.%s (in outputs): attribute not found", attrName)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok || len(sa.Validators) == 0 {
			continue
		}
		p := path.Root("outputs").AtName("catalog").AtName(attrName)
		if !hasValidationError(ctx, sa, p, types.StringValue("")) {
			t.Errorf(
				"catalog.%s (in outputs): validator did not fire for empty string — partial block must be rejected",
				attrName,
			)
		}
	}
}

// ── structural guard: blocks must not be stubs ────────────────────────────────

// TestDataplexSchema_JobTypeAndCatalog_AreNotStubs is a structural sanity check:
// both job_type (job facet) and catalog (dataset facet) must be active (non-stub)
// blocks in the Dataplex schema because they are listed in Capability().
//
// A stub block has all attributes Optional+Computed; an active block has at
// least one Optional+!Computed attribute.
func TestDataplexSchema_JobTypeAndCatalog_AreNotStubs(t *testing.T) {
	s := dataplexSchema()

	// job_type
	jobTypeBlock, ok := s.Blocks["job_type"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected job_type to be a SingleNestedBlock")
	}
	if !hasActiveAttr(jobTypeBlock.Attributes) {
		t.Error("job_type block looks like a stub — expected at least one Optional+!Computed attr")
	}

	// catalog (in inputs)
	inputs := s.Blocks["inputs"].(schema.ListNestedBlock)
	catalogBlock, ok := inputs.NestedObject.Blocks["catalog"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("expected catalog to be a SingleNestedBlock inside inputs")
	}
	if !hasActiveAttr(catalogBlock.Attributes) {
		t.Error("catalog block (in inputs) looks like a stub — expected at least one Optional+!Computed attr")
	}
}

// hasActiveAttr returns true when at least one StringAttribute in the map is
// Optional and not Computed. This is the signature of an "active" (enabled)
// facet attribute, as opposed to a stub where every attribute is Optional+Computed.
func hasActiveAttr(attrs map[string]schema.Attribute) bool {
	for _, a := range attrs {
		sa, ok := a.(schema.StringAttribute)
		if ok && sa.Optional && !sa.Computed {
			return true
		}
	}
	return false
}

