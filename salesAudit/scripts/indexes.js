// Sales Audit indexes. Safe to run more than once (createIndex is a no-op when the index exists).
//   mongosh "$MONGO_URI/$MONGO_DATABASE" salesAudit/scripts/indexes.js

// Feature collections
db.salesAuditRechecks.createIndex({ program: 1, id: 1 }, { unique: true })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, leadId: 1, raisedAt: -1 })
db.salesAuditRechecks.createIndex({ program: 1, deleted: 1, status: 1 })

db.salesAuditAlerts.createIndex({ program: 1, id: 1 }, { unique: true })
db.salesAuditAlerts.createIndex({ program: 1, deleted: 1, leadId: 1, kind: 1, sentAt: 1 })
db.salesAuditAlerts.createIndex({ program: 1, deleted: 1, sentAt: 1 })

db.salesAuditCcResponses.createIndex({ program: 1, leadId: 1 }, { unique: true })
db.salesAuditAudits.createIndex({ program: 1, leadId: 1 }, { unique: true })
db.salesAuditCcExtracts.createIndex({ program: 1, leadId: 1 }, { unique: true })

// Read paths on the Zoho collections (indexes only; their documents are never modified)
db.LeadData.createIndex({ Stage: 1, deleted: 1 })
db.LeadData.createIndex({ ID: 1 })
db.paymentData.createIndex({ All_Enrolment: 1 })
db.paymentData.createIndex({ Zen_ID: 1 })
db.EmiData.createIndex({ Zen_ID: 1 })
db.PartialReminders.createIndex({ Student_ID: 1 })
db.SubscriptionReminders.createIndex({ Zen_ID: 1 })
db.DiscountData.createIndex({ Learner_Email_ID: 1 })

print('Sales Audit indexes are in place.')
