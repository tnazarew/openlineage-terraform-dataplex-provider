provider "openlineage" {
  project_id = "my-gcp-project"
  region     = "us-central1"

  # Optional: path to a GCP service account key file.
  # If omitted, Application Default Credentials (ADC) are used.
  # credentials_file = "/path/to/service-account.json"
}

