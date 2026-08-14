package terraform

// ResourceSchemas is the stable Phase 7 resource surface. The production
// Terraform provider can bind these names to the Terraform Plugin Framework.
var ResourceSchemas = []string{
	"dbvault_organisation", "dbvault_project", "dbvault_environment",
	"dbvault_agent_token", "dbvault_destination", "dbvault_repository",
	"dbvault_source", "dbvault_schedule", "dbvault_policy",
	"dbvault_notification_route",
}

var DataSourceSchemas = []string{
	"dbvault_agent", "dbvault_backup", "dbvault_recovery_window", "dbvault_destination_health",
}
