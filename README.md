# OpenLineage Terraform Provider — Dataplex

A Terraform provider that emits [OpenLineage](https://openlineage.io) events to the
[GCP Dataplex Lineage API](https://cloud.google.com/dataplex/docs/lineage-overview),
creating and managing lineage entities (Process, Run, LineageEvent) as Terraform resources.

This is an implementation of the **Bring Your Own OpenLineage (BYOOL)** concept — declaring
static lineage for pipelines that don't have runtime OpenLineage instrumentation.

The generic resource building blocks live in the OpenLineage monorepo at
[`byool/terraform`](https://github.com/OpenLineage/openlineage/tree/main/byool/terraform).

---

## How It Works

```
  .tf config                Provider                    GCP Dataplex
 ──────────────     ───────────────────────────     ──────────────────────
                            │
  openlineage_job  ──────►  │  1. Build OL RunEvent
    namespace                │     (job + inputs +
    name                     │      outputs + facets)
    inputs  ──────────────►  │
    outputs ──────────────►  │  2. ProcessOpenLineageRunEvent (gRPC)
                            │ ─────────────────────────────────────────►
                            │                                    creates / updates:
                            │ ◄─────────────────────────────────────────
                            │     process_name                  • Process
                            │     run_name                      • Run
                            │     lineage_event_name            • LineageEvent
                            │
                            │  3. ListRuns → get run_state
                            │ ─────────────────────────────────────────►
                            │ ◄───────────────────── "COMPLETED" ───────
                            │
  terraform.tfstate ◄──────  │  4. Save state
    process_name             │
    run_name                 │
    lineage_event_name       │
    run_state                │
    run_id                   │
```

### Resource Lifecycle

Each `openlineage_job` resource maps to a **Dataplex Process** entity. The Process
is the stable long-lived entity that represents a recurring job. Each `terraform apply`
creates a new **Run** under the same Process, recording a new lineage event.

```
terraform apply (first time)
  └─ emit RunEvent  →  Dataplex creates Process + Run + LineageEvent
                       store process_name, run_name, lineage_event_name in state

terraform apply (config changed)
  └─ emit new RunEvent  →  Dataplex reuses same Process, creates new Run + LineageEvent
                           update run_id, run_name, lineage_event_name in state

terraform plan (refresh)
  └─ GET process_name   →  still exists?  →  refresh run_state
                        →  404 (deleted outside TF)?  →  mark for re-create (drift)

terraform destroy
  └─ DeleteProcess  →  Dataplex removes Process (and all its Runs)
```

---

## Requirements

- Terraform ≥ 1.0
- A GCP project with the [Dataplex API enabled](https://console.cloud.google.com/apis/library/datalineage.googleapis.com)
- One of:
  - Application Default Credentials (`gcloud auth application-default login`)
  - A service account key file with `roles/datalineage.admin`

---

## Installation

### From Terraform Registry

```hcl
terraform {
  required_providers {
    openlineage = {
      source  = "tnazarew/openlineage-dataplex"
      version = "~> 0.0.1"
    }
  }
}
```

### From source (development)

```bash
git clone https://github.com/tnazarew/terraform-provider-openlineage-dataplex
cd terraform-provider-openlineage-dataplex
make build   # builds binary to ./bin/
make plan    # build + terraform plan in ./examples/
make apply   # build + terraform apply in ./examples/
```

---

## Provider Configuration

```hcl
provider "openlineage" {
  project_id   = "my-gcp-project"   # required
  region       = "us-central1"      # required
  # credentials_file = "/path/to/sa.json"  # optional, uses ADC if omitted

  warn_on_unused_facets = true  # optional, default true
                                # set false when migrating from another consumer
}
```

### Provider Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | ✅ | GCP project ID |
| `region` | string | ✅ | GCP region where Dataplex Lineage API is enabled |
| `credentials_file` | string | | Path to a service account JSON key. Omit to use ADC. |
| `warn_on_unused_facets` | bool | | Emit warnings when config defines facets Dataplex ignores. Default: `true`. Set `false` during migrations. |

### Environment Variable Fallbacks

| Variable | Overridden by |
|---|---|
| `GCP_PROJECT_ID` | `project_id` in provider block |
| `GCP_REGION` | `region` in provider block |
| `GOOGLE_APPLICATION_CREDENTIALS` | `credentials_file` in provider block |

---

## Resource: `openlineage_job`

Manages a lineage job in GCP Dataplex. Each resource instance corresponds to one
Dataplex **Process** entity. Applying emits an OpenLineage `RunEvent` (`COMPLETE`)
which Dataplex processes into Process + Run + LineageEvent entities.

### Example — minimal

```hcl
resource "openlineage_job" "example" {
  namespace = "airflow"
  name      = "analytics.aggregate_sales"

  inputs {
    namespace = "bigquery"
    name      = "my-project.raw.orders"
  }

  outputs {
    namespace = "bigquery"
    name      = "my-project.analytics.sales_summary"
  }
}
```

### Example — with facets and column lineage

```hcl
resource "openlineage_job" "aggregate_sales" {
  namespace   = "airflow"
  name        = "analytics.aggregate_product_sales"
  description = "Joins products and orders to produce sales statistics"

  inputs {
    namespace = "bigquery"
    name      = "my-project.raw.products"

    symlinks {
      namespace = "hive"
      name      = "default.products"
      type      = "TABLE"
    }
  }

  inputs {
    namespace = "bigquery"
    name      = "my-project.raw.order_items"
  }

  outputs {
    namespace = "bigquery"
    name      = "my-project.analytics.product_sales"

    column_lineage {
      fields {
        name = "total_revenue"

        input_field {
          namespace = "bigquery"
          name      = "my-project.raw.order_items"
          field     = "unit_price"

          transformation {
            type        = "DIRECT"
            subtype     = "AGGREGATION"
            description = "Sum of unit prices"
          }
        }
      }

      dataset {
        namespace = "bigquery"
        name      = "my-project.raw.products"
        field     = "product_id"

        transformation {
          type    = "INDIRECT"
          subtype = "FILTER"
        }
      }
    }
  }
}
```

### Schema Reference

#### Top-level arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | OpenLineage job namespace. Changing this forces replacement. |
| `name` | string | ✅ | OpenLineage job name. Changing this forces replacement. |
| `description` | string | | Free-text description attached to the OL job. |

#### `inputs` block (repeatable)

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | Dataset namespace (e.g. `bigquery`) |
| `name` | string | ✅ | Dataset name (e.g. `project.dataset.table`) |
| `symlinks` | block | | Alternate identifiers — see below |
| `catalog` | block | | Catalog/metastore metadata — see below |

#### `outputs` block (repeatable)

Same arguments as `inputs`, plus `column_lineage`.

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | Dataset namespace |
| `name` | string | ✅ | Dataset name |
| `symlinks` | block | | Alternate identifiers |
| `catalog` | block | | Catalog/metastore metadata |
| `column_lineage` | block | | Field-to-field lineage — see below |

#### `symlinks` block

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | Alternate namespace |
| `name` | string | ✅ | Alternate name |
| `type` | string | ✅ | e.g. `TABLE`, `VIEW` |

#### `column_lineage` block (on `outputs` only)

**`fields` sub-block:**

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | ✅ | Output column name |
| `input_field` | block | | Contributing input columns |

**`input_field` sub-block:**

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | Input dataset namespace |
| `name` | string | ✅ | Input dataset name |
| `field` | string | ✅ | Input column name |
| `transformation` | block | | How this field was transformed |

**`dataset` sub-block:**

| Argument | Type | Required | Description |
|---|---|---|---|
| `namespace` | string | ✅ | Input dataset namespace |
| `name` | string | ✅ | Input dataset name |
| `field` | string | ✅ | Output field this dataset contributes to |
| `transformation` | block | | How the data was transformed |

**`transformation` sub-block:**

| Argument | Type | Required | Description |
|---|---|---|---|
| `type` | string | ✅ | `DIRECT` or `INDIRECT` |
| `subtype` | string | | e.g. `IDENTITY`, `AGGREGATION`, `FILTER` |
| `description` | string | | Human-readable explanation |
| `masking` | bool | | `true` if this transform masks/anonymises data |

#### Computed attributes (read-only)

| Attribute | Description |
|---|---|
| `id` | Internal identifier: `namespace.name` |
| `run_id` | UUID generated for the most recent emission |
| `process_name` | Full GCP resource name of the Dataplex Process |
| `run_name` | Full GCP resource name of the most recent Dataplex Run |
| `lineage_event_name` | Full GCP resource name of the most recent LineageEvent |
| `run_state` | State of the most recent Run: `COMPLETED`, `FAILED`, `RUNNING` |
| `update_time` | Timestamp of the most recent Run end time (ISO 8601) |

---

## Drift Detection

During `terraform plan`, the provider calls `GetProcess` with the stored `process_name`:

- **Process still exists** → refresh `run_state` and `update_time`, no change planned
- **Process deleted outside Terraform (404)** → resource removed from state, plan shows `+` (re-create)

---

## Import

An existing Dataplex Process can be imported into Terraform state:

```bash
terraform import openlineage_job.example airflow:my.pipeline
```

The import ID format is `namespace:job_name`.

---

## Development

### Build & test locally

```bash
make build   # compiles to ./bin/
make plan    # build + terraform plan in ./examples/
make apply   # build + terraform apply in ./examples/
make destroy # build + terraform destroy in ./examples/
make docs    # regenerates ./docs/ from schema + templates
```

### Project Structure

```
main.go                      Provider entry point
internal/dataplex/
  provider.go                Provider config schema and authentication
  dataplex_job_resource.go   openlineage_job CRUD + schema
  dataplex_client.go         GCP Dataplex API calls
  dataplex_models.go         Dataplex computed state structs
examples/
  provider/provider.tf       Minimal provider config example
  resources/openlineage_job/ Resource example (used by tfplugindocs)
docs/                        Generated — do not edit manually
templates/                   tfplugindocs templates
```

---

## Related

- [OpenLineage](https://openlineage.io) — the lineage specification
- [OpenLineage Go client](https://github.com/OpenLineage/openlineage/tree/main/client/go) — used for event construction
- [BYOOL Terraform resources](https://github.com/OpenLineage/openlineage/tree/main/byool/terraform) — generic resource building blocks used by this provider
- [GCP Dataplex Lineage API](https://cloud.google.com/dataplex/docs/lineage-overview)
