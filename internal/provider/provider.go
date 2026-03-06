package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure OpenLineageProvider satisfies various provider interfaces.
var _ provider.Provider = &OpenLineageProvider{}

// OpenLineageProvider defines the provider implementation.
type OpenLineageProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// OpenLineageProviderModel describes the provider data model.
type OpenLineageProviderModel struct {
	ProjectID       types.String `tfsdk:"project_id"`
	Region          types.String `tfsdk:"region"`
	CredentialsFile types.String `tfsdk:"credentials_file"`
}

func (p *OpenLineageProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openlineage"
	resp.Version = p.version
}

func (p *OpenLineageProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for emitting OpenLineage events to GCP Lineage API",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "GCP Project ID for lineage emissions",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "GCP Region for lineage API",
				Required:    true,
			},
			"credentials_file": schema.StringAttribute{
				Description: "Path to GCP credentials JSON file. If not set, will use Application Default Credentials",
				Optional:    true,
			},
		},
	}
}

func (p *OpenLineageProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config OpenLineageProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	if config.ProjectID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("project_id"),
			"Unknown GCP Project ID",
			"The provider cannot create the OpenLineage client as there is an unknown configuration value for the GCP project ID.",
		)
	}

	if config.Region.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("region"),
			"Unknown GCP Region",
			"The provider cannot create the OpenLineage client as there is an unknown configuration value for the GCP region.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	projectID := os.Getenv("GCP_PROJECT_ID")
	region := os.Getenv("GCP_REGION")
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	if !config.ProjectID.IsNull() {
		projectID = config.ProjectID.ValueString()
	}

	if !config.Region.IsNull() {
		region = config.Region.ValueString()
	}

	if !config.CredentialsFile.IsNull() {
		credentialsFile = config.CredentialsFile.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.
	if projectID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("project_id"),
			"Missing GCP Project ID",
			"The provider cannot create the OpenLineage client as there is a missing or empty value for the GCP project ID.",
		)
	}

	if region == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("region"),
			"Missing GCP Region",
			"The provider cannot create the OpenLineage client as there is a missing or empty value for the GCP region.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Create configuration for resource initialization
	cfg := &ProviderConfig{
		ProjectID:       projectID,
		Region:          region,
		CredentialsFile: credentialsFile,
	}

	// Make the configuration available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = cfg
	resp.ResourceData = cfg
}

func (p *OpenLineageProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewJobResource,
	}
}

func (p *OpenLineageProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// No datasources in simplified architecture
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &OpenLineageProvider{
			version: version,
		}
	}
}

// ProviderConfig holds the configuration for OpenLineage provider
type ProviderConfig struct {
	ProjectID       string
	Region          string
	CredentialsFile string
}
