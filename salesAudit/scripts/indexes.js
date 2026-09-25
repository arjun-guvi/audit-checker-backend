// Sales Audit indexes. Safe to run more than once (createIndex is a no-op when the index exists).
//   mongosh "$MONGO_URI/$MONGO_DATABASE" salesAudit/scripts/indexes.js

const collections = [
  'salesAuditLeads',
  'salesAuditMembers',
  'salesAuditAudits',
  'salesAuditRechecks',
  'salesAuditEvents',
  'salesAuditNotifications',
  'salesAuditCounters',
  'salesAuditAlerts',
  'salesAuditCcExtracts',
]
for (const name of collections) db[name].createIndex({ program: 1, id: 1 }, { unique: true })

// Leads: one per Zoho enrolment; list filters
db.salesAuditLeads.createIndex({ program: 1, zenId: 1 }, { unique: true })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, 'assignment.auditorEmail': 1, 'audit.status': 1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, bdaEmail: 1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, bdmEmail: 1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, region: 1, 'audit.status': 1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, 'cc.status': 1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, 'audit.completedAt': -1 })
db.salesAuditLeads.createIndex({ program: 1, deleted: 1, enrolledAt: -1, crmCreatedAt: -1 })

// Members: sign-in by user hash (or dev email)
db.salesAuditMembers.createIndex({ program: 1, email: 1 }, { unique: true })
db.salesAuditMembers.createIndex({ program: 1, userHash: 1 })

// Audits: many attempts per lead (re-audit), so leadId is not unique. The old unique index blocked it.
if (db.salesAuditAudits.getIndexes().some((index) => index.name === 'program_1_leadId_1' && index.unique)) {
  db.salesAuditAudits.dropIndex('program_1_leadId_1')
}
db.salesAuditAudits.createIndex({ program: 1, deleted: 1, leadId: 1, submittedAt: -1 })
db.salesAuditAudits.createIndex({ program: 1, deleted: 1, 'auditor.email': 1, submittedAt: -1 })

// Rechecks: ticket number, list filters
db.salesAuditRechecks.createIndex({ program: 1, recheckNo: 1 }, { unique: true })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, leadId: 1, raisedAt: -1 })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, status: 1, 'closed.at': -1 })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, bdaEmail: 1, status: 1 })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, bdmEmail: 1, status: 1 })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, auditorEmail: 1, status: 1 })

// Timeline, notifications, counters, mail log, CC extracts
db.salesAuditEvents.createIndex({ program: 1, deleted: 1, leadId: 1, at: 1 })
db.salesAuditNotifications.createIndex({ program: 1, deleted: 1, recipientEmail: 1, read: 1, 'created.at': -1 })
db.salesAuditCounters.createIndex({ program: 1, name: 1 }, { unique: true })
db.salesAuditAlerts.createIndex({ program: 1, deleted: 1, leadId: 1, kind: 1, sentAt: -1 })
db.salesAuditAlerts.createIndex({ program: 1, deleted: 1, sentAt: -1 })
db.salesAuditCcExtracts.createIndex({ program: 1, leadId: 1 }, { unique: true })

print('Sales Audit indexes are in place.')
