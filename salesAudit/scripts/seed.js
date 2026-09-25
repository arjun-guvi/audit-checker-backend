// Fake Sales Audit members for a local/dev database (no real PII). Re-running replaces them.
//   mongosh "mongodb://localhost:27017/audit_app" salesAudit/scripts/seed.js
//
// Sign in from the dev shell with the token "dev-mock-token:<email>" of any member below.
// Leads come from Zoho: generate a fake Zoho response and import it as the TL:
//   node salesAudit/scripts/fakeZohoResponse.js 100 > /tmp/zoho.json
//   curl -X POST http://localhost:8080/sales-audit/zoho/import \
//     -H "Authorization: dev-mock-token:tl@example.com" --data-binary @/tmp/zoho.json
const PROGRAM = 'guvi'
const now = Math.floor(Date.now() / 1000)
const member = (email, name, role, region, managerEmail) => ({
  id: require('crypto').randomUUID(),
  program: PROGRAM,
  userHash: `dev-mock-token:${email}`,
  email,
  name,
  role,
  region,
  managerEmail,
  available: true,
  created: { at: now, by: 'seed' },
  deleted: false,
})

const members = [
  member('tl@example.com', 'Audit TL', 'auditorTl', '', ''),
  member('auditor.north@example.com', 'North Auditor', 'auditor', 'North', 'tl@example.com'),
  member('auditor.south1@example.com', 'South Auditor One', 'auditor', 'South', 'tl@example.com'),
  member('auditor.south2@example.com', 'South Auditor Two', 'auditor', 'South', 'tl@example.com'),
  member('bdm.north@example.com', 'North BDM', 'bdm', '', ''),
  member('bdm.south@example.com', 'South BDM', 'bdm', '', ''),
  member('bda.north1@example.com', 'North BDA One', 'bda', '', 'bdm.north@example.com'),
  member('bda.north2@example.com', 'North BDA Two', 'bda', '', 'bdm.north@example.com'),
  member('bda.south1@example.com', 'South BDA One', 'bda', '', 'bdm.south@example.com'),
  member('bda.south2@example.com', 'South BDA Two', 'bda', '', 'bdm.south@example.com'),
]

db.salesAuditMembers.deleteMany({ program: PROGRAM, email: { $in: members.map((m) => m.email) } })
db.salesAuditMembers.insertMany(members)
print(`Seeded ${members.length} Sales Audit members for program ${PROGRAM}.`)
