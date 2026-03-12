# Bring Your Own OpenLineage — Terraform Provider Proposal

**Author**: Tomasz Nazarewicz  
**Date**: March 10, 2026  
**Related**: Go client PR #4358  
**Reference implementation**: [openlineage-terraform-dataplex-provider](https://github.com/tnazarew/openlineage-terraform-dataplex-provider)

---

## Background

There is a well-established idea called **Bring Your Own Lineage (BYOL)** — enabling users
to declare lineage entities explicitly rather than relying on runtime instrumentation.
Databricks already ships this concept for Unity Catalog
([external lineage](https://docs.databricks.com/aws/en/data-governance/unity-catalog/external-lineage)).

OpenLineage is uniquely positioned to take this further: because it defines a
transport-agnostic, consumer-agnostic lineage format, lineage defined once in OpenLineage
can be replayed to any compatible catalog. **Bring Your Own OpenLineage (BYOOL)** is the
natural name for this capability applied to the OL ecosystem.

The gap this fills: components that have no runtime OpenLineage integration (legacy ETL,
stored procedures, notebooks, manually curated pipelines) currently produce no lineage at all.
BYOOL lets their owners declare lineage statically, without modifying the components themselves.

---

## Why Terraform

Terraform is the dominant infrastructure-as-code tool. It manages the lifecycle of declared
entities through a well-defined state model that maps directly onto what BYOOL needs:

| Terraform concept | BYOOL meaning |
|---|---|
| **config** | Desired lineage state declared by the user in `.tf` files |
| **previous** (state file) | Last known state of the lineage entity in the catalog |
| **current** (live system) | Actual state of the entity in the lineage catalog right now |
| **apply** | Emit the OL event that creates or updates the entity |
| **destroy** | Remove the entity from the catalog |
| **drift detection** | Detect if the entity was changed or deleted outside of Terraform |

Users already manage infrastructure with Terraform. Lineage declarations living alongside
infrastructure definitions is a natural fit — especially for data pipelines where the
infrastructure (tables, jobs, schedules) and the lineage (what reads from what) change together.

---

## The Core Problem: Config is Generic, State is Consumer-Specific

The OpenLineage event format (the **config** side) is fully generic and transport-agnostic.
However, the **state** returned after emitting an event is entirely consumer-specific:

- **Dataplex** returns GCP resource names: `process_name`, `run_name`, `lineage_event_name`
- **Marquez** returns UUIDs and REST paths
- **Atlan** returns asset GUIDs

This means a single universal provider cannot exist. Each lineage catalog requires its own
provider, because:
1. The state schema differs per consumer
2. Drift detection (Read) uses consumer-specific APIs
3. Delete uses consumer-specific APIs
4. Some consumers only support certain event types (e.g. Dataplex only accepts `RunEvent`)

**However**, the configuration structure — the OpenLineage event definition — is identical
across all consumers. This is the shared module that belongs in the OpenLineage repository.

---

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────────┐
│              openlineage/client/go/pkg/terraform            │  ← IN THIS REPO
│                                                              │
│  ol/                                                         │
│    capability.go      Facet enum, Capability struct,         │
│                       capability constructors                │
│    models.go          JobFacets, DatasetFacets — all tfsdk   │
│                       structs mirroring the OL spec          │
│    schema_generator.go  SchemaGenerator.Generate(cap)        │
│    event_builder.go   buildEvent(model, cap) → RunEvent      │
│                                                              │
│  consumer/                                                   │
│    consumer.go        Consumer interface                     │
│    state/             Reference implementation (no network)  │
└──────────────────────────────┬──────────────────────────────┘
                               │  imported by
        ┌──────────────────────┼───────────────────┐
        ▼                      ▼                   ▼
┌──────────────┐   ┌────────────────────┐   ┌──────────────┐
│   Dataplex   │   │      Marquez       │   │    Atlan     │
│   provider   │   │      provider      │   │   provider   │
│  (separate   │   │    (separate       │   │  (separate   │
│    repo)     │   │      repo)         │   │    repo)     │
└──────────────┘   └────────────────────┘   └──────────────┘
```

### What lives in this repository (`openlineage/client/go`)

The shared module provides everything that is consumer-independent:

#### 1. OL config models (`ol/models.go`)

Terraform `tfsdk`-tagged structs that mirror the OL spec exactly.
These are the structs users fill in their `.tf` files.
They cover **job facets** and **dataset facets** — the parts of the spec
that are meaningful as static declarations:

**Job facets** (from `facets.JobFacets`):
- `JobType` — processing type, integration, job type classification
- `Ownership` — owners list with name and type
- `Documentation` — human-readable job description
- `SourceCode` — language and code content
- `SourceCodeLocation` — VCS location (git URL, path, branch, tag)
- `SQL` — SQL query text
- `Tags` — free-form key/value tags

**Dataset facets** (from `facets.DatasetFacets`):
- `Symlinks` — alternate names/namespaces for this dataset
- `Schema` — column definitions (name, type, description)
- `DataSource` — source system name and URI
- `Documentation` — human-readable dataset description
- `DatasetType` — TABLE, VIEW, STREAM, storage layer, media type
- `Version` — dataset version at time of event
- `Storage` — physical storage layer and file format
- `Ownership` — dataset owners
- `LifecycleStateChange` — CREATE, DROP, ALTER, RENAME, OVERWRITE
- `Hierarchy` — parent/child relationships (e.g. partition within a table)
- `Catalog` — metastore registration (Hive, Iceberg, etc.)
- `ColumnLineage` — field-to-field lineage mappings with transformation metadata
- `Tags` — free-form key/value tags

Run facets, input-dataset-only facets, and output-dataset-only facets are
intentionally **excluded** from the static config — they describe runtime
execution characteristics that static lineage declarations do not have.

#### 2. Capability system (`ol/capability.go`)

A facet enum and a `Capability` struct that controls which facets appear in the
Terraform schema for a given consumer. Each consumer declares its capability as
a base event type with a set of disabled facets:

```go
// Base constructors — consumers extend these
func RunEventCapability() Capability  // full RunEvent, nothing disabled
func JobEventCapability() Capability  // RunEvent with run facets disabled
func DatasetEventCapability() Capability // JobEvent with job facets also disabled

// Consumers extend a base and disable what they don't support
func DataplexCapability() Capability {
    return RunEventCapability().WithFacetDisabled(
        FacetJobType, FacetOwnership, FacetSQL, FacetSourceCode,
        FacetCatalog, FacetSchema, FacetDataSource,
        // ...
    )
}
```

Key design decisions:
- **Facets are all-or-nothing** — if a facet is enabled, its full OL spec structure
  is exposed. Individual fields within a facet are never trimmed.
- **`FieldMode`** — each facet can be `Active` (user can set, triggers plan diff),
  `Inert` (user can set, no diff — for migration from another consumer), or
  `Disabled` (excluded from schema).
- **`warn_on_unused_facets`** — a provider-level flag (default: `true`) that emits
  Terraform warnings when the user defines facets the active consumer ignores.
  Set to `false` during migrations to suppress noise.

#### 3. Schema generator (`ol/schema_generator.go`)

A single `SchemaGenerator.Generate(cap Capability) schema.Schema` function that
builds the complete Terraform resource schema from a capability declaration.
Each consumer's `Schema()` method becomes a one-liner:

```go
func (r *JobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = ol.SchemaGenerator.Generate(DataplexCapability())
}
```

The generator conditionally includes each facet block based on the capability,
so consumers with different capabilities automatically get different schemas
without writing any schema construction code.

#### 4. Event builder (`ol/event_builder.go`)

`buildEvent(model *OLConfig, cap Capability, runID uuid.UUID) *openlineage.RunEvent`

Translates the Terraform model into an OL RunEvent, attaching only the facets
that are not disabled in the capability. Consumer-specific facets (e.g. Dataplex's
`GcpLineage` origin facet) are injected by the consumer's `Emit()` method, not here.

#### 5. Consumer interface (`consumer/consumer.go`)

```go
type Consumer interface {
    Capability() ol.Capability
    Emit(ctx context.Context, event *openlineage.RunEvent) (*EmitResult, error)
    Read(ctx context.Context, primaryID string) (*ResourceState, error)
    Delete(ctx context.Context, primaryID string) error
}
```

#### 6. State-only reference consumer (`consumer/state/`)

The simplest possible consumer — no network calls. Derives a stable ID from
`namespace:name`, stores config in Terraform state, always reports as existing.
Useful for:
- Testing the schema and event builder without a real catalog
- Demos and development
- Reference implementation showing the minimum a consumer must implement

---

## What Each Consumer Provider Implements

Each consumer is a separate repository/binary. It imports the shared module and provides:

| Component | Lines of code (estimate) | What it does |
|---|---|---|
| `capability.go` | ~20 | Declares which facets this consumer supports |
| `resource_job.go` | ~50 | `Schema()` one-liner + CRUD wiring |
| `client.go` | varies | Consumer API calls (emit, read, delete) |
| `provider.go` | ~80 | Provider config schema + consumer instantiation |

The bulk of the code — models, schema construction, event building — lives once
in the shared module.

---

## Event Type Mapping

Not all consumers support all OL event types. The capability system handles this:

| Resource | OL event emitted | Notes |
|---|---|---|
| `openlineage_job` | `RunEvent` | Dataplex only accepts RunEvent. Job info is in the Run wrapper. |
| `openlineage_job` | `JobEvent` | Future — for consumers that support static job events |
| `openlineage_dataset` | `DatasetEvent` | Future — for dataset-only lineage |

The event type is determined by the consumer's base capability:
- `RunEventCapability()` → emits `RunEvent`
- `JobEventCapability()` → emits `RunEvent` with run-scoped facets stripped (effectively a job event)
- `DatasetEventCapability()` → emits `RunEvent` with run and job facets stripped

---

## Terraform Resource Lifecycle (Dataplex example)

```
terraform apply
    │
    ├─ Plan phase
    │   ├─ Read: GET process by stored process_name
    │   │        if missing → mark for create (drift detected)
    │   │        if origin.name != provider → warn (not owned by this provider)
    │   └─ Diff: compare config hash with previous state
    │            if changed → mark for update
    │
    └─ Apply phase
        ├─ Create / Update:
        │   ├─ Generate fresh run UUID
        │   ├─ ol.buildEvent(config, capability, runID)
        │   ├─ consumer.Emit(event)  →  ProcessOpenLineageRunEvent
        │   │   returns: process_name, run_name, lineage_event_name
        │   └─ Store in state: process_name, run_name, lineage_event_name,
        │                      run_state, update_time, run_id
        │
        └─ Delete:
            └─ consumer.Delete(process_name)  →  DeleteProcess (LRO)
```

State stored in `terraform.tfstate` per resource instance:

```json
{
  "id": "airflow:my.pipeline",
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "ol": {
    "job": { "namespace": "airflow", "name": "my.pipeline" },
    "job_facets": { "job_type": { "processing_type": "BATCH", "integration": "BYOL", "job_type": "JOB" } },
    "inputs":  [{ "namespace": "bigquery", "name": "project.dataset.input_table", "facets": {} }],
    "outputs": [{ "namespace": "bigquery", "name": "project.dataset.output_table", "facets": {
      "column_lineage": { "fields": [{ "name": "out_col", "input_field": [{ "namespace": "bigquery", "name": "project.dataset.input_table", "field": "in_col", "transformation": [{ "type": "DIRECT" }] }] }] }
    }}]
  },
  "dataplex": {
    "process_name": "projects/123/locations/us/processes/abc",
    "run_name": "projects/123/locations/us/processes/abc/runs/xyz",
    "lineage_event_name": "projects/123/locations/us/lineageEvents/def",
    "run_state": "COMPLETED",
    "update_time": "2026-03-10T12:00:00Z"
  }
}
```

---

## Example Terraform Configuration

```hcl
provider "openlineage" {
  project_id          = "my-gcp-project"
  region              = "us-central1"
  warn_on_unused_facets = true  # default — warns when config defines facets the consumer ignores
}

resource "openlineage_job" "aggregate_sales" {
  ol {
    job {
      namespace   = "airflow"
      name        = "analytics.aggregate_product_sales"
      description = "Joins products and orders to produce sales statistics"
    }

    job_facets {
      job_type {
        processing_type = "BATCH"
        integration     = "BYOL"
        job_type        = "JOB"
      }
    }

    inputs {
      namespace = "bigquery"
      name      = "my-project.raw.products"
    }

    inputs {
      namespace = "bigquery"
      name      = "my-project.raw.order_items"
    }

    outputs {
      namespace = "bigquery"
      name      = "my-project.analytics.product_sales"

      facets {
        column_lineage {
          fields {
            name = "total_revenue"
            input_field {
              namespace = "bigquery"
              name      = "my-project.raw.order_items"
              field     = "unit_price"
              transformation { type = "DIRECT" subtype = "AGGREGATION" }
            }
          }
        }
      }
    }
  }
}
```

---

## Migration Between Consumers

A key design goal is that lineage config should survive moving from one catalog to another.
The `warn_on_unused_facets = false` flag suppresses warnings during migration:

```hcl
provider "openlineage" {
  # switching from marquez to dataplex
  project_id            = "my-gcp-project"
  region                = "us-central1"
  warn_on_unused_facets = false  # suppress warnings while cleaning up Marquez-specific config
}
```

Facets declared as `FieldInert` in a consumer's capability appear in the schema and
accept values but never trigger a plan diff — so users can keep them in their `.tf`
files without Terraform constantly wanting to update the resource.

---

## Requirements

1. **Go client** — the shared module uses the OL Go client (PR #4358) for event construction
   and transport. The Terraform `tfsdk` struct tags in the models are the only Terraform
   dependency in the shared module.

2. **Terraform Plugin Framework** — each consumer provider uses
   `github.com/hashicorp/terraform-plugin-framework`. The shared `ol/` module itself only
   depends on the `tfsdk` struct tags (a compile-time annotation), not on any Terraform
   runtime library.

3. **Consumer-specific dependencies** — each consumer provider adds its own dependencies
   (e.g. GCP client libraries for Dataplex, HTTP client for Marquez).

---

## Suggested Repository Location

```
client/go/
  pkg/
    openlineage/   ← existing Go client
    facets/        ← existing facets
    transport/     ← existing transports
    terraform/     ← NEW — shared BYOOL Terraform module
      ol/
        capability.go
        models.go
        schema_generator.go
        event_builder.go
      consumer/
        consumer.go
        state/
          consumer.go    ← reference implementation
```

This keeps the Terraform module close to the Go client it depends on and makes it
straightforward to keep facet models in sync with `facets.gen.go` as the OL spec evolves.

---

## Reference Implementation

The [OpenLineage Terraform Dataplex provider](https://github.com/tnazarew/openlineage-terraform-dataplex-provider)
is a working implementation of a consumer provider. It demonstrates:

- The complete resource lifecycle (Create, Read, Update, Delete, Import)
- Drift detection via `GetProcess` + origin verification
- Event emission via `ProcessOpenLineageRunEvent`
- Run state polling via `ListRuns`
- The capability/schema pattern (currently hardcoded, to be refactored to use the shared module)
- The `warn_on_unused_facets` provider flag
- Column lineage, symlinks, and catalog facet support

The provider successfully emits lineage events to GCP Dataplex and creates
Process/Run/LineageEvent entities visible in the Dataplex Lineage UI.

