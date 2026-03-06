# Terraform Provider for OpenLineage (Simplified Architecture)

A Terraform provider for emitting OpenLineage events to GCP Dataplex Lineage API.

## Architecture

This provider uses a **simplified single-resource architecture** with a **dual-state model**:

- **User Configuration**: Define jobs using OpenLineage specification (namespace, name, inputs, outputs, metadata)
- **Computed State**: Track Dataplex Process state (process_name, run_id, run_state, timestamps)

### Design Principles

1. **Simplicity**: Single `openlineage_job` resource instead of separate dataset/job resources
2. **Declarative**: Jobs are defined with simple dataset references (no complex nesting)
3. **Integration**: Direct integration with GCP Dataplex Lineage API
4. **Flexibility**: Easy to extend with additional facets as needed

## Installation

```hcl
terraform {
  required_providers {
    openlineage = {
      source  = "local/openlineage"
      version = "0.1.0"
    }
  }
}
```

## Provider Configuration

```hcl
provider "openlineage" {
  project_id       = "my-gcp-project"
  region           = "us-central1"
  credentials_file = "/path/to/credentials.json"  # Optional, uses ADC if not set
}
```

### Configuration Options

- `project_id` (Required): GCP Project ID for Dataplex lineage
- `region` (Required): GCP Region for Dataplex API
- `credentials_file` (Optional): Path to GCP credentials. Uses Application Default Credentials if not set.

Environment variables:

- `GCP_PROJECT_ID`: Default project ID
- `GCP_REGION`: Default region
- `GOOGLE_APPLICATION_CREDENTIALS`: Default credentials file

## Resource: `openlineage_job`

Defines a job that emits OpenLineage RunEvents to Dataplex.

### Example Usage

```hcl
resource "openlineage_job" "transform_sales" {
  namespace   = "my-scheduler"
  name        = "etl_pipeline.transform_sales"
  description = "Transforms raw sales data"

  job_type {
    processing_type = "BATCH"
    integration     = "SPARK"
    job_type        = "JOB"
  }

  owners {
    name = "team:data-engineering"
    type = "MAINTAINER"
  }

  inputs {
    namespace = "bigquery://my-project"
    name      = "raw_data.sales"
  }

  outputs {
    namespace = "bigquery://my-project"
    name      = "analytics.sales_summary"
  }
}
```

### Schema

#### User Configuration (Required/Optional)

- `namespace` (Required): Job namespace (e.g., `my-scheduler-namespace`)
- `name` (Required): Job name (e.g., `etl_pipeline.transform_data`)
- `description` (Optional): Job description

**Blocks:**

- `job_type` (Optional): Job type metadata
    - `processing_type` (Required): `BATCH` or `STREAMING`
    - `integration` (Required): Integration type (e.g., `SPARK`, `AIRFLOW`, `DBT`, `BYOL`)
    - `job_type` (Required): Job type (e.g., `QUERY`, `COMMAND`, `DAG`, `TASK`, `JOB`, `MODEL`)

- `owners` (Optional, repeatable): Job ownership
    - `name` (Required): Owner identifier (e.g., `team:data-engineering`)
    - `type` (Required): Owner type (e.g., `MAINTAINER`, `OWNER`)

- `inputs` (Optional, repeatable): Input dataset references
    - `namespace` (Required): Dataset namespace
    - `name` (Required): Dataset name

- `outputs` (Optional, repeatable): Output dataset references
    - `namespace` (Required): Dataset namespace
    - `name` (Required): Dataset name

#### Computed Dataplex State (Read-only)

- `id`: Internal identifier (`namespace.name`)
- `process_name`: Dataplex process resource name
- `run_id`: Latest run UUID
- `run_state`: Latest run state (e.g., `COMPLETED`, `FAILED`)
- `creation_time`: Process creation timestamp (ISO 8601)
- `update_time`: Process last update timestamp (ISO 8601)

### Behavior

**On Create/Update:**

1. Generates a new run UUID
2. Emits an OpenLineage RunEvent (COMPLETE) to Dataplex
3. Reads back Dataplex Process state
4. Updates computed attributes

**On Read:**

- Refreshes computed state from Dataplex API

**On Delete:**

- Deletes the Dataplex Process resource

## Development Status

This is a **skeleton implementation**. The following are not yet implemented:

- [ ] Actual OpenLineage event building (waiting for Go client library)
- [ ] Actual Dataplex API calls (GET/DELETE operations)
- [ ] Error handling for API responses
- [ ] Retry logic for eventual consistency

The provider currently logs operations but does not make real API calls. This allows:

- Schema validation and testing
- Terraform plan/apply dry runs
- Parallel development of Go OpenLineage client

See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) for detailed status.

## Examples

See the [examples/](./examples/) directory for complete examples:

- `simple_job.tf`: Basic job definition with inputs/outputs

## Building

```bash
go build -o terraform-provider-openlineage
```

## Testing

```bash
go test ./...
```

## Architecture Documentation

- [ARCHITECTURE_EXPLAINED.md](ARCHITECTURE_EXPLAINED.md): Detailed architecture decisions
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md): Current implementation status

## Contributing

This provider is under active development. The simplified architecture allows for:

1. Independent Go OpenLineage client development
2. Gradual Dataplex API integration
3. Easy extension with additional facets

## License

[Your License Here]
