BINARY_NAME=terraform-provider-openlineage
INSTALL_DIR=bin
OS_ARCH=$(shell go env GOOS)_$(shell go env GOARCH)
TF_ENV=TF_CLI_CONFIG_FILE=$(PWD)/.terraformrc
GOPATH_BIN=$(shell go env GOPATH)/bin

# Path to local OL client — only needed on dev machine
OL_CLIENT_PATH=/Users/tomasznazarewicz/projects/OpenLineage/client/go

.PHONY: build build-vendor install clean plan apply destroy test vendor env docs docs-install
# Build the provider binary
build:
	mkdir -p $(INSTALL_DIR)
	go build -o $(INSTALL_DIR)/$(BINARY_NAME) .
	@echo "✅ Built: $(INSTALL_DIR)/$(BINARY_NAME)"

# Build using vendored dependencies (portable, no local OL client needed)
build-vendor:
	mkdir -p $(INSTALL_DIR)
	go build -mod=vendor -o $(INSTALL_DIR)/$(BINARY_NAME) .
	@echo "✅ Built (vendor): $(INSTALL_DIR)/$(BINARY_NAME)"

# Vendor all dependencies so the repo is self-contained and portable.
# Run this after any change to the OL client, then commit vendor/.
vendor:
	@echo "Vendoring dependencies from local OL client at $(OL_CLIENT_PATH)..."
	go mod tidy
	go mod vendor
	@echo "✅ vendor/ created — commit this directory to make the repo portable"
	@echo ""
	@echo "On another machine, build with: make build-vendor"

# Run terraform apply using dev_overrides
show:
	cd examples && $(TF_ENV) terraform show

# Run terraform plan using dev_overrides (no init needed)
plan: build
	@echo "Using dev_overrides — 'terraform init' is not required."
	cd examples && $(TF_ENV) terraform plan

# Run terraform apply using dev_overrides
apply: build
	cd examples && $(TF_ENV) terraform apply -auto-approve

# Destroy all managed resources
destroy: build
	cd examples && $(TF_ENV) terraform destroy -auto-approve

# Clean build artifacts and terraform state
clean:
	rm -rf $(INSTALL_DIR)
	rm -f examples/terraform.tfstate examples/terraform.tfstate.backup
	rm -rf examples/.terraform

# Install into the local Terraform plugin cache (alternative to dev_overrides)
install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/tomasznazarewicz/openlineage/0.1.0/$(OS_ARCH)
	cp $(INSTALL_DIR)/$(BINARY_NAME) ~/.terraform.d/plugins/registry.terraform.io/tomasznazarewicz/openlineage/0.1.0/$(OS_ARCH)/
	@echo "✅ Installed to ~/.terraform.d/plugins/"

# Run tests
test:
	go test ./... -v

# Generate provider documentation from templates + schema
docs:
	$(GOPATH_BIN)/tfplugindocs generate --provider-name openlineage
	@echo "✅ Docs generated in docs/"

# Install tfplugindocs if not present
docs-install:
	go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
	@echo "✅ tfplugindocs installed"

# Show which env vars can configure the provider
env:
	@echo "Provider reads these environment variables:"
	@echo "  GCP_PROJECT_ID                  — GCP project ID"
	@echo "  GCP_REGION                      — GCP region (default: us-central1)"
	@echo "  GOOGLE_APPLICATION_CREDENTIALS  — path to service account JSON (optional, uses ADC if unset)"
