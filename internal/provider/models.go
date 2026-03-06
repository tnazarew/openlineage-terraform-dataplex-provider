package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

// JobResourceModel is the central data structure for the openlineage_job resource.
//
// It has two distinct sections:
//   - "User Configuration" fields: written by the Terraform user in their .tf file,
//     used to build the OpenLineage RunEvent that gets emitted to Dataplex.
//   - "Dataplex State" fields: computed automatically by the provider after the API
//     call — the user never sets these, they're read back from Dataplex and stored
//     in terraform.tfstate so they can be referenced as outputs.
//
// The `tfsdk:"..."` tags must exactly match the attribute/block names in the
// schema defined in resource_job.go — Terraform uses them to map HCL ↔ struct.
type JobResourceModel struct {
	// ── User Configuration (written in the .tf file) ──────────────────────────

	// ID is a synthetic identifier we construct as "namespace.name".
	// It's Computed (never set by the user) and kept stable via UseStateForUnknown.
	ID types.String `tfsdk:"id"`

	// Namespace is the OpenLineage job namespace (e.g. "airflow", "spark").
	// Marked RequiresReplace — changing it destroys the old Dataplex Process
	// and creates a new one, because the Process identity in Dataplex is tied
	// to the namespace+name combination from the OL event.
	Namespace types.String `tfsdk:"namespace"`

	// Name is the OpenLineage job name (e.g. "analytics.my_pipeline").
	// Also RequiresReplace for the same reason as Namespace.
	Name types.String `tfsdk:"name"`

	// Description is a free-text description attached to the OL job facet.
	Description types.String `tfsdk:"description"`

	// Inputs is the list of input datasets this job reads from.
	// Each entry becomes an InputElement in the OpenLineage RunEvent.
	Inputs []InputModel `tfsdk:"inputs"`

	// Outputs is the list of output datasets this job writes to.
	// Each entry becomes an OutputElement in the OpenLineage RunEvent.
	Outputs []OutputModel `tfsdk:"outputs"`

	// JobType holds optional job classification metadata (BATCH/STREAMING, integration, etc.)
	// It maps to the OpenLineage JobTypeJobFacet.
	// Stored as types.Object because it's a single optional nested block.
	JobType types.Object `tfsdk:"job_type"`

	// Owners is a list of people/teams responsible for this job.
	// Maps to the OpenLineage OwnershipJobFacet.
	Owners types.List `tfsdk:"owners"`

	// ── Dataplex State (computed by the provider, read from the API) ───────────

	// ProcessName is the full GCP resource name of the Dataplex Process entity,
	// e.g. "projects/my-project/locations/us-central1/processes/abc123".
	// Dataplex creates this automatically when we emit the first OL event.
	// We store it so that Read and Delete can reference the exact resource.
	ProcessName types.String `tfsdk:"process_name"`

	// RunID is the UUID we generated locally for this specific emission.
	// It's part of the OpenLineage event, not a Dataplex resource name.
	RunID types.String `tfsdk:"run_id"`

	// RunName is the full GCP resource name of the Dataplex Run entity,
	// e.g. "projects/.../processes/.../runs/xyz".
	// Returned by ProcessOpenLineageRunEvent in the response.
	RunName types.String `tfsdk:"run_name"`

	// LineageEventName is the full GCP resource name of the Dataplex LineageEvent
	// entity created for this specific emission.
	// Also returned directly by ProcessOpenLineageRunEvent.
	LineageEventName types.String `tfsdk:"lineage_event_name"`

	// RunState is the current state of the latest Run (e.g. "COMPLETED", "FAILED").
	// Fetched separately via ListRuns after emission since it may take a moment to settle.
	RunState types.String `tfsdk:"run_state"`

	// CreationTime was removed — Dataplex does not return it from ProcessOpenLineageRunEvent
	// and GetProcess does not expose a creation timestamp in the API response.
	UpdateTime types.String `tfsdk:"update_time"`
}

// InputModel represents a single input dataset declared in the `inputs {}` block.
// It maps to an OpenLineage InputElement, which describes a dataset the job reads from.
type InputModel struct {
	Namespace types.String   `tfsdk:"namespace"` // e.g. "bigquery"
	Name      types.String   `tfsdk:"name"`      // e.g. "my-project.raw.products"
	Symlinks  []SymlinkModel `tfsdk:"symlinks"`  // alternate identifiers for this dataset
	Catalog   []CatalogModel `tfsdk:"catalog"`   // catalog/metastore metadata (at most one entry used)
}

// OutputModel represents a single output dataset declared in the `outputs {}` block.
// It maps to an OpenLineage OutputElement, which describes a dataset the job writes to.
// Outputs can additionally carry column-level lineage information.
type OutputModel struct {
	Namespace     types.String         `tfsdk:"namespace"`
	Name          types.String         `tfsdk:"name"`
	Symlinks      []SymlinkModel       `tfsdk:"symlinks"`
	Catalog       []CatalogModel       `tfsdk:"catalog"`
	ColumnLineage []ColumnLineageModel `tfsdk:"column_lineage"` // field-to-field lineage mappings
}

// SymlinkModel represents an alternate name/location for a dataset.
// Useful when the same dataset is known under different identifiers in different systems
// (e.g. the same table exists in both BigQuery and Hive).
// Maps to an entry in the OpenLineage SymlinksDatasetFacet.
type SymlinkModel struct {
	Namespace types.String `tfsdk:"namespace"` // the system where this alternate name lives
	Name      types.String `tfsdk:"name"`      // the alternate dataset name
	Type      types.String `tfsdk:"type"`      // e.g. "TABLE", "VIEW"
}

// CatalogModel carries catalog/metastore metadata about a dataset.
// Maps to the OpenLineage CatalogDatasetFacet.
// Only the first entry in the list is used (the schema allows multiple for HCL flexibility,
// but the OL spec has one catalog facet per dataset).
type CatalogModel struct {
	Framework    types.String `tfsdk:"framework"`     // e.g. "hive", "iceberg"
	Type         types.String `tfsdk:"type"`          // e.g. "hive"
	MetadataURI  types.String `tfsdk:"metadata_uri"`  // e.g. "hive://localhost:9083"
	WarehouseURI types.String `tfsdk:"warehouse_uri"` // e.g. "hdfs://localhost/tmp/warehouse"
	Source       types.String `tfsdk:"source"`        // e.g. "spark" (optional)
}

// ColumnLineageModel holds the full column lineage declaration for an output dataset.
// It has two sub-types:
//   - Fields: maps a specific output column → its contributing input fields
//   - Dataset: captures dataset-level lineage (input dataset contributes to an output field,
//     without knowing the exact input column)
type ColumnLineageModel struct {
	Fields  []ColumnLineageFieldModel   `tfsdk:"fields"`  // output-column → input-field mappings
	Dataset []ColumnLineageDatasetModel `tfsdk:"dataset"` // dataset-level contribution entries
}

// ColumnLineageFieldModel maps a single output column to the input fields that produce it.
// In the OpenLineage spec, this becomes a key in the ColumnLineageFacet.fields map,
// where the key is the output column name and the value is a FieldValue containing
// the list of contributing input fields.
type ColumnLineageFieldModel struct {
	Name        types.String      `tfsdk:"name"`        // the output column name (becomes the map key)
	InputFields []InputFieldModel `tfsdk:"input_field"` // which input columns feed this output column
}

// InputFieldModel represents a single input column that contributes to an output column.
// Maps to a DatasetElement inside a FieldValue in the OpenLineage ColumnLineageFacet.
type InputFieldModel struct {
	Namespace      types.String          `tfsdk:"namespace"`      // the input dataset's namespace
	Name           types.String          `tfsdk:"name"`           // the input dataset's name
	Field          types.String          `tfsdk:"field"`          // the specific column in that dataset
	Transformation []TransformationModel `tfsdk:"transformation"` // how the field was transformed (optional)
}

// ColumnLineageDatasetModel represents a dataset-level lineage entry — where we know
// an input dataset contributes to an output field, but don't know the exact input column.
// Also maps to a DatasetElement in the OpenLineage ColumnLineageFacet.
type ColumnLineageDatasetModel struct {
	Namespace      types.String          `tfsdk:"namespace"`
	Name           types.String          `tfsdk:"name"`
	Field          types.String          `tfsdk:"field"`
	Transformation []TransformationModel `tfsdk:"transformation"`
}

// TransformationModel describes how an input field is transformed to produce an output field.
// Maps to the OpenLineage Transformation type inside a DatasetElement.
type TransformationModel struct {
	Type        types.String `tfsdk:"type"`        // "DIRECT" or "INDIRECT" (required)
	Subtype     types.String `tfsdk:"subtype"`     // e.g. "IDENTITY", "FILTER", "AGGREGATION" (optional)
	Description types.String `tfsdk:"description"` // human-readable explanation (optional)
	Masking     types.Bool   `tfsdk:"masking"`     // true if the transformation masks/anonymises data (optional)
}

// JobTypeModel carries job classification metadata.
// Maps to the OpenLineage JobTypeJobFacet.
type JobTypeModel struct {
	ProcessingType types.String `tfsdk:"processing_type"` // "BATCH" or "STREAMING"
	Integration    types.String `tfsdk:"integration"`     // e.g. "SPARK", "AIRFLOW", "DBT"
	JobType        types.String `tfsdk:"job_type"`        // e.g. "QUERY", "DAG", "TASK", "JOB"
}

// OwnerModel represents a single owner entry for the job.
// Maps to an entry in the OpenLineage OwnershipJobFacet owners list.
type OwnerModel struct {
	Name types.String `tfsdk:"name"` // e.g. "team:data-engineering" or "user:alice"
	Type types.String `tfsdk:"type"` // e.g. "MAINTAINER", "OWNER", "STEWARD"
}
