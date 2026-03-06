package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Compile-time check: ensures JobResource implements the resource.Resource interface.
// If we forget to implement a required method, this line will produce a build error.
var _ resource.Resource = &JobResource{}

func NewJobResource() resource.Resource {
	return &JobResource{}
}

// JobResource is the implementation of the openlineage_job Terraform resource.
//
// The Terraform Plugin Framework calls its methods in this sequence:
//
//	Configure → Schema → Create / Read / Update / Delete
//
// JobResource holds the Dataplex API client and GCP config that are shared
// across all CRUD operations. They are injected by Configure() when Terraform
// initialises the provider.
type JobResource struct {
	dpClient  *dataplexClient // GCP Lineage API client — does all actual API calls
	projectID string          // GCP project, passed through to the client
	region    string          // GCP region, passed through to the client
}

func (r *JobResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	// TypeName becomes the resource address in HCL: "<provider>_job"
	// e.g. if provider is "openlineage" → resource "openlineage_job"
	resp.TypeName = req.ProviderTypeName + "_job"
}

// Schema defines every attribute and block the user can write in their .tf file,
// as well as every computed attribute Terraform will read back from the API.
//
// Sub-schemas (transformationSchema, symlinkSchema, catalogSchema) are defined
// as local variables and reused across inputs and outputs to avoid repetition.
// Terraform validates the user's config against this schema before calling Create/Update.
func (r *JobResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// transformationSchema is shared by input_field and dataset blocks.
	// It describes how a data field was transformed between input and output.
	transformationSchema := schema.ListNestedBlock{
		Description: "Transformation applied to the field",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required:    true,
					Description: "Transformation type: DIRECT or INDIRECT",
				},
				"subtype": schema.StringAttribute{
					Optional:    true,
					Description: "Transformation subtype (e.g., IDENTITY, FILTER)",
				},
				"description": schema.StringAttribute{
					Optional:    true,
					Description: "Human-readable description of the transformation",
				},
				"masking": schema.BoolAttribute{
					Optional:    true,
					Description: "Whether the transformation masks the data",
				},
			},
		},
	}

	// symlinkSchema is shared by inputs and outputs.
	// Symlinks declare that a dataset is also known under a different name/namespace.
	symlinkSchema := schema.ListNestedBlock{
		Description: "Alternate dataset identifiers (symlinks)",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"namespace": schema.StringAttribute{
					Required:    true,
					Description: "Symlink namespace",
				},
				"name": schema.StringAttribute{
					Required:    true,
					Description: "Symlink name",
				},
				"type": schema.StringAttribute{
					Required:    true,
					Description: "Symlink type (e.g., TABLE)",
				},
			},
		},
	}

	// catalogSchema is shared by inputs and outputs.
	// It carries metastore/catalog metadata (e.g. Hive URIs, framework type).
	catalogSchema := schema.ListNestedBlock{
		Description: "Dataset catalog metadata",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"framework": schema.StringAttribute{
					Required:    true,
					Description: "Catalog framework (e.g., hive)",
				},
				"type": schema.StringAttribute{
					Required:    true,
					Description: "Catalog type (e.g., hive)",
				},
				"metadata_uri": schema.StringAttribute{
					Required:    true,
					Description: "URI of the catalog metadata service",
				},
				"warehouse_uri": schema.StringAttribute{
					Required:    true,
					Description: "URI of the data warehouse",
				},
				"source": schema.StringAttribute{
					Optional:    true,
					Description: "Source system (e.g., spark)",
				},
			},
		},
	}

	resp.Schema = schema.Schema{
		Description: "OpenLineage job resource that emits RunEvents to GCP Dataplex Lineage API.",

		Attributes: map[string]schema.Attribute{
			// id is computed by the provider (namespace.name) and never set by the user.
			// UseStateForUnknown preserves the value across plan/apply cycles so Terraform
			// doesn't show "(known after apply)" on every plan.
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Internal identifier (namespace.name)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// namespace and name together identify the job in OpenLineage AND determine
			// which Dataplex Process this resource maps to.
			// RequiresReplace means: if either changes, Terraform will Delete the old
			// Process and Create a new one (rather than calling Update).
			"namespace": schema.StringAttribute{
				Required:    true,
				Description: "Job namespace",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Job name",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Job description",
			},

			// ── Computed fields — set by the provider from the Dataplex API response ──
			//
			// The user never writes these in their .tf file.
			// After Create/Update, we store the GCP resource names returned by
			// ProcessOpenLineageRunEvent so they can be used as Terraform outputs
			// and referenced by other resources.
			//
			// process_name uses UseStateForUnknown so that it persists across plans
			// where the config hasn't changed (avoids unnecessary "(known after apply)").
			"process_name": schema.StringAttribute{
				Computed:    true,
				Description: "Dataplex process resource name (returned by ProcessOpenLineageRunEvent)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"run_id": schema.StringAttribute{
				Computed:    true,
				Description: "UUID of the latest emitted run",
			},
			"run_name": schema.StringAttribute{
				Computed:    true,
				Description: "Dataplex run resource name (returned by ProcessOpenLineageRunEvent)",
			},
			"lineage_event_name": schema.StringAttribute{
				Computed:    true,
				Description: "Dataplex lineage event resource name (returned by ProcessOpenLineageRunEvent)",
			},
			"run_state": schema.StringAttribute{
				Computed:    true,
				Description: "Latest run state from Dataplex (COMPLETED, FAILED, etc.)",
			},
			"update_time": schema.StringAttribute{
				Computed:    true,
				Description: "Process last update time (ISO 8601)",
			},
		},

		Blocks: map[string]schema.Block{
			"job_type": schema.SingleNestedBlock{
				Description: "Job type information",
				Attributes: map[string]schema.Attribute{
					"processing_type": schema.StringAttribute{
						Optional:    true,
						Description: "Processing type: BATCH or STREAMING",
					},
					"integration": schema.StringAttribute{
						Optional:    true,
						Description: "Integration type (e.g., SPARK, AIRFLOW, DBT, BYOL)",
					},
					"job_type": schema.StringAttribute{
						Optional:    true,
						Description: "Job type: QUERY, COMMAND, DAG, TASK, JOB, MODEL",
					},
				},
			},
			"owners": schema.ListNestedBlock{
				Description: "Job owners",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Owner name (e.g., team:data-engineering)",
						},
						"type": schema.StringAttribute{
							Required:    true,
							Description: "Owner type (e.g., MAINTAINER, OWNER)",
						},
					},
				},
			},
			"inputs": schema.ListNestedBlock{
				Description: "Input datasets",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"namespace": schema.StringAttribute{
							Required:    true,
							Description: "Dataset namespace",
						},
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Dataset name",
						},
					},
					Blocks: map[string]schema.Block{
						"symlinks": symlinkSchema,
						"catalog":  catalogSchema,
					},
				},
			},
			"outputs": schema.ListNestedBlock{
				Description: "Output datasets",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"namespace": schema.StringAttribute{
							Required:    true,
							Description: "Dataset namespace",
						},
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Dataset name",
						},
					},
					Blocks: map[string]schema.Block{
						"symlinks": symlinkSchema,
						"catalog":  catalogSchema,
						"column_lineage": schema.ListNestedBlock{
							Description: "Column-level lineage for this output dataset",
							NestedObject: schema.NestedBlockObject{
								Blocks: map[string]schema.Block{
									"fields": schema.ListNestedBlock{
										Description: "Output field mappings",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"name": schema.StringAttribute{
													Required:    true,
													Description: "Output column name",
												},
											},
											Blocks: map[string]schema.Block{
												"input_field": schema.ListNestedBlock{
													Description: "Input fields that contribute to this output field",
													NestedObject: schema.NestedBlockObject{
														Attributes: map[string]schema.Attribute{
															"namespace": schema.StringAttribute{
																Required:    true,
																Description: "Input dataset namespace",
															},
															"name": schema.StringAttribute{
																Required:    true,
																Description: "Input dataset name",
															},
															"field": schema.StringAttribute{
																Required:    true,
																Description: "Input column name",
															},
														},
														Blocks: map[string]schema.Block{
															"transformation": transformationSchema,
														},
													},
												},
											},
										},
									},
									"dataset": schema.ListNestedBlock{
										Description: "Dataset-level lineage entries",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"namespace": schema.StringAttribute{
													Required:    true,
													Description: "Dataset namespace",
												},
												"name": schema.StringAttribute{
													Required:    true,
													Description: "Dataset name",
												},
												"field": schema.StringAttribute{
													Required:    true,
													Description: "Field name",
												},
											},
											Blocks: map[string]schema.Block{
												"transformation": transformationSchema,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure is called by Terraform after the provider is initialised.
// It receives the ProviderConfig (project, region, credentials) that the user
// set in the `provider "openlineage" {}` block, creates the Dataplex API client,
// and stores both on the JobResource struct so Create/Read/Update/Delete can use them.
func (r *JobResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// ProviderData is nil during planning before the provider is configured.
	// Returning early here is correct and expected.
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*ProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderConfig, got: %T", req.ProviderData),
		)
		return
	}

	r.projectID = config.ProjectID
	r.region = config.Region

	// Create a single shared Dataplex client for this resource instance.
	// The client opens a gRPC connection here — we reuse it for all operations.
	dpClient, err := newDataplexClient(ctx, config.ProjectID, config.Region, config.CredentialsFile)
	if err != nil {
		resp.Diagnostics.AddError("Dataplex Client Error", fmt.Sprintf("Unable to create Dataplex client: %s", err))
		return
	}
	r.dpClient = dpClient
}

// Create is called by Terraform on the first `terraform apply` for this resource.
//
// What it does:
//  1. Reads the planned config from req.Plan into a JobResourceModel
//  2. Generates a fresh run UUID (unique per apply)
//  3. Builds an OpenLineage RunEvent from the model (via event_builder.go)
//  4. Calls emitAndCapture() — sends the event to Dataplex, gets back resource names
//  5. Calls refreshRunState() — fetches the run_state from the new Run
//  6. Saves everything to Terraform state via resp.State.Set()
func (r *JobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data JobResourceModel

	// req.Plan contains what the user wrote in their .tf file (after interpolation).
	// We decode it into our JobResourceModel struct using the tfsdk tags.
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build a stable ID from namespace + name. This is what Terraform uses as the
	// resource's primary key in the state file.
	data.ID = types.StringValue(fmt.Sprintf("%s.%s", data.Namespace.ValueString(), data.Name.ValueString()))

	// Generate a fresh UUID for this specific emission.
	// Each terraform apply creates a new Run in Dataplex under the same Process.
	// The UUID is embedded in the OL event as the run identifier.
	runID := uuid.New()
	data.RunID = types.StringValue(runID.String())

	tflog.Info(ctx, "Creating job resource", map[string]any{
		"id":        data.ID.ValueString(),
		"namespace": data.Namespace.ValueString(),
		"name":      data.Name.ValueString(),
		"run_id":    runID.String(),
	})

	// Build the OL event struct and emit it to Dataplex via ProcessOpenLineageRunEvent.
	// AsEmittable() converts the RunEvent to the form that can be JSON-marshalled
	// (the OL client uses an internal representation internally).
	event := buildRunEvent(&data, runID)
	result, err := r.dpClient.emitAndCapture(ctx, event.AsEmittable())
	if err != nil {
		resp.Diagnostics.AddError("Emission Error", fmt.Sprintf("Unable to emit event: %s", err))
		return
	}

	// Store the GCP resource names returned directly from the API response.
	// This is more reliable than searching for them afterwards.
	data.ProcessName = types.StringValue(result.ProcessName)
	data.RunName = types.StringValue(result.RunName)
	if len(result.LineageEventNames) > 0 {
		// The API can return multiple lineage event names (one per input/output pair),
		// but we store just the first as a representative reference.
		data.LineageEventName = types.StringValue(result.LineageEventNames[0])
	}

	tflog.Info(ctx, "Event emitted successfully", map[string]any{
		"run_id":       runID.String(),
		"process_name": result.ProcessName,
		"run_name":     result.RunName,
	})

	// Fetch the run_state and update_time from the newly created Run.
	// This is a separate API call because ProcessOpenLineageRunEvent doesn't return state.
	r.refreshRunState(ctx, &data)

	// Persist everything to the Terraform state file.
	// After this, `terraform.tfstate` will contain all the computed fields.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is called by Terraform during `terraform plan` and `terraform refresh`
// to sync the state with the real world (drift detection).
//
// What it does:
//  1. Loads the previously saved state (process_name, etc.) from req.State
//  2. Calls GetProcess(process_name) — the fastest way to check if the Process still exists
//  3. If the Process is gone (404): calls RemoveResource() so Terraform knows to re-create it
//  4. If it still exists: refreshes run_state and update_time from the latest Run
func (r *JobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data JobResourceModel

	// req.State is the last known state — what was saved in terraform.tfstate.
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Reading job resource", map[string]any{
		"id":           data.ID.ValueString(),
		"process_name": data.ProcessName.ValueString(),
	})

	processName := data.ProcessName.ValueString()
	if processName == "" {
		// No process_name stored yet (e.g. state was partially created).
		// Nothing to verify — just keep the current state as-is.
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	// Verify the Dataplex Process still exists by fetching it directly by name.
	// This is O(1) — much cheaper than scanning all processes.
	process, err := r.dpClient.getProcess(ctx, processName)
	if err != nil {
		resp.Diagnostics.AddError("Read Error", fmt.Sprintf("Unable to read Dataplex process: %s", err))
		return
	}

	if process == nil {
		// The process was deleted outside of Terraform (drift detected).
		// RemoveResource() removes it from the state file, which causes Terraform
		// to show a "+" (create) action on the next plan.
		tflog.Warn(ctx, "Dataplex process no longer exists, removing from state", map[string]any{
			"process_name": processName,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	// If the process exists but wasn't created by this provider, warn but don't fail.
	// This can happen if a process with the same name was created manually or by
	// another tool — OriginVerified=false means origin.name doesn't match providerOriginName.
	if !process.OriginVerified {
		tflog.Warn(ctx, "Dataplex process origin does not match provider — process may not have been created by this provider", map[string]any{
			"process_name": processName,
			"origin_name":  process.OriginName,
			"expected":     providerOriginName,
		})
	}

	// Process still exists — refresh the volatile fields (run_state, update_time)
	// in case the latest Run has finished or its state has changed.
	r.refreshRunState(ctx, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is called when the user changes any attribute that is NOT marked RequiresReplace.
// (name and namespace are RequiresReplace, so changing them triggers Delete+Create instead.)
//
// In practice, Update is triggered by changes to description, inputs, outputs, job_type, owners.
// It re-emits the full OL RunEvent with the new config, which creates a new Run under
// the same existing Dataplex Process (the Process persists across updates).
func (r *JobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data JobResourceModel

	// req.Plan has the new desired state; req.State has the previous state.
	// We read from Plan so we apply the user's latest config.
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// New UUID per update — each apply creates a distinct Run in Dataplex.
	runID := uuid.New()
	data.RunID = types.StringValue(runID.String())

	tflog.Info(ctx, "Updating job resource", map[string]any{
		"id":        data.ID.ValueString(),
		"namespace": data.Namespace.ValueString(),
		"name":      data.Name.ValueString(),
		"run_id":    runID.String(),
	})

	// Build OpenLineage event and emit via Dataplex, capturing returned resource names
	event := buildRunEvent(&data, runID)
	result, err := r.dpClient.emitAndCapture(ctx, event.AsEmittable())
	if err != nil {
		resp.Diagnostics.AddError("Emission Error", fmt.Sprintf("Unable to emit event: %s", err))
		return
	}

	// Update stored names from the response
	data.ProcessName = types.StringValue(result.ProcessName)
	data.RunName = types.StringValue(result.RunName)
	if len(result.LineageEventNames) > 0 {
		data.LineageEventName = types.StringValue(result.LineageEventNames[0])
	}

	tflog.Info(ctx, "Event emitted successfully", map[string]any{
		"run_id":       runID.String(),
		"process_name": result.ProcessName,
		"run_name":     result.RunName,
	})

	// Refresh run state
	r.refreshRunState(ctx, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete is called on `terraform destroy` or when a resource is removed from the config.
//
// It deletes the Dataplex Process by its stored resource name, which also removes all
// child Runs and LineageEvents. After this method returns without error, Terraform
// automatically removes the resource from the state file.
//
// Note: when name/namespace changes, Terraform calls Delete (on the old resource)
// then Create (for the new one) — this is what RequiresReplace achieves.
func (r *JobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data JobResourceModel

	// req.State has the last known state — we need the process_name from it.
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Deleting job resource", map[string]any{
		"id":           data.ID.ValueString(),
		"process_name": data.ProcessName.ValueString(),
	})

	// Delete the Dataplex process if we have a process name
	processName := data.ProcessName.ValueString()
	if processName != "" && r.dpClient != nil {
		if err := r.dpClient.deleteProcess(ctx, processName); err != nil {
			resp.Diagnostics.AddError("Delete Error",
				fmt.Sprintf("Unable to delete Dataplex process %s: %s", processName, err))
			return
		}
		tflog.Info(ctx, "Dataplex process deleted", map[string]any{
			"process_name": processName,
		})
	}
}

// refreshRunState fetches the latest Run for the process stored in state and
// updates the run_state and update_time fields in the model.
//
// It's called after every Create/Update (to confirm the Run reached a final state)
// and during Read (to detect if the state has changed since the last apply).
//
// This is a best-effort call — if it fails (e.g. network error) we log a warning
// but don't fail the operation, since the emit already succeeded.
func (r *JobResource) refreshRunState(ctx context.Context, data *JobResourceModel) {
	if r.dpClient == nil || data.ProcessName.ValueString() == "" {
		return
	}

	run, err := r.dpClient.getLatestRun(ctx, data.ProcessName.ValueString())
	if err != nil {
		tflog.Warn(ctx, "Failed to query Dataplex runs", map[string]any{
			"error":        err.Error(),
			"process_name": data.ProcessName.ValueString(),
		})
		return
	}

	if run == nil {
		// Process exists but no runs yet — can happen briefly after emission.
		return
	}

	data.RunState = types.StringValue(run.State)
	// Prefer EndTime (the run finished); fall back to StartTime if it's still running.
	if !run.EndTime.IsZero() {
		data.UpdateTime = types.StringValue(run.EndTime.Format(time.RFC3339))
	} else if !run.StartTime.IsZero() {
		data.UpdateTime = types.StringValue(run.StartTime.Format(time.RFC3339))
	}

	tflog.Info(ctx, "Run state refreshed", map[string]any{
		"process_name": data.ProcessName.ValueString(),
		"run_state":    run.State,
	})
}
