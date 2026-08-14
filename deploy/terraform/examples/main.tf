terraform {
  required_providers {
    dbvault = {
      source = "dbvault/dbvault"
      version = "~> 0.7"
    }
  }
}
provider "dbvault" {
  endpoint = var.dbvault_endpoint
  token    = var.dbvault_token
}
resource "dbvault_repository" "production" {
  organisation_id = var.organisation_id
  project_id      = var.project_id
  name            = "production"
  mode            = "primary_replica"
}
