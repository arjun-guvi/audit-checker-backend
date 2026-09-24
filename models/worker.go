package models

const (
	NAMESPACE = "audit_app"

	CONCURRENT_TASKS = 10

	// Job names
	HELLO_WORLD_JOB = "hello_world_job"

	// Sales Audit jobs
	SALES_AUDIT_ESCALATION_SWEEP_JOB       = "salesAudit_sap_escalation_sweep"
	SALES_AUDIT_RECHECK_REMINDER_SWEEP_JOB = "salesAudit_recheck_reminder_sweep"
	SALES_AUDIT_SEND_MAIL_JOB              = "salesAudit_send_mail"
)
