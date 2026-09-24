// Fake data for a local/dev database only (no real PII). Re-running replaces the previous seed:
// every seeded Zoho record has an ID starting with "SEED-", and seeded feature documents use the
// program below.
//   mongosh "mongodb://localhost:27017/audit_app_dev" scripts/sales_audit_seed.js
const PROGRAM = 'guvi'

const pad = (n) => String(n).padStart(2, '0')
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
// Zoho writes "15-Sep-2026 15:22:19"
const zohoTime = (date) =>
  `${pad(date.getDate())}-${MONTHS[date.getMonth()]}-${date.getFullYear()} ` +
  `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
const zohoDate = (date) => zohoTime(date).slice(0, 11)
const hoursAgo = (hours) => new Date(Date.now() - hours * 3600 * 1000)
const nowSeconds = () => Math.floor(Date.now() / 1000)
const newId = () => require('crypto').randomUUID()

const bda1 = 'Sales Owner One - owner1@example.com'
const bda2 = 'Sales Owner Two - owner2@example.com'
const bdm = 'Sales Manager A - managera@example.com'

const lead = (n, fields) => ({
  ID: `SEED-L${n}`,
  zen_id: `SEED-Z${n}`,
  Stage: 'Audit',
  Student_Full_Name: `Seed Learner ${n}`,
  Email: `learner${n}@example.com`,
  Primary_Phone: `+9100000000${pad(n)}`,
  Sale_Owner: bda1,
  Sale_Owner_s_Manager: bdm,
  EMI_Status: 'Not Applied',
  Mode_of_Study: 'Online - Weekday',
  Preferred_Language: 'English',
  'Assigned_Batch.Start_Date': '15-Oct-2026',
  Confirmation_Call_Link: '',
  Confirmation_Call_Added_Date_Time: '',
  Discount_Given: '',
  Partial_Split_Up_Category: '',
  deleted: false,
  ...fields,
})

const leads = [
  // Unverified down payment, 60h in SAP -> escalation due
  lead(1, {
    Course: 'Full Stack Development', Course_Value: '78800', Payment_Type: 'Direct - Full Payment',
    Total_Paid: '999.00', Balance_Amount: '77801.00', Added_Time: zohoTime(hoursAgo(60)),
  }),
  // Partial plan with CC uploaded and a mismatched initial payment
  lead(2, {
    Course: 'UI/UX Program', Course_Value: '75000', Payment_Type: 'Direct - Partial Payment',
    Partial_Split_Up_Category: '40-30-30', Total_Paid: '30999.00', Balance_Amount: '44001.00',
    Sale_Owner: bda2, Added_Time: zohoTime(hoursAgo(30)), Discount_Given: '5000',
    Confirmation_Call_Link: 'https://example.com/cc/seed-2', Confirmation_Call_Added_Date_Time: zohoTime(hoursAgo(4)),
  }),
  // EMI lead with a vendor record
  lead(3, {
    Course: 'Data Science', Course_Value: '18000', Payment_Type: 'EMI - 10 Month', EMI_Status: 'Disbursed',
    Total_Paid: '999.00', Balance_Amount: '0.00', Sale_Owner: bda2, Added_Time: zohoTime(hoursAgo(8)),
  }),
  // Subscription, nothing paid yet (not escalated), CC pending
  lead(4, {
    Course: 'Full Stack Development', Course_Value: '9000', Payment_Type: 'Subscription',
    Partial_Split_Up_Category: '25-25-25-25', Total_Paid: '0.00', Balance_Amount: '9000.00',
    Added_Time: zohoTime(hoursAgo(12)),
  }),
  // Everything verified
  lead(5, {
    Course: 'UI/UX Program', Course_Value: '73800', Payment_Type: 'Direct - Full Payment',
    Total_Paid: '73800.00', Balance_Amount: '0.00', Added_Time: zohoTime(hoursAgo(40)),
    Confirmation_Call_Link: 'https://example.com/cc/seed-5', Confirmation_Call_Added_Date_Time: zohoTime(hoursAgo(20)),
  }),
  // Another stage: reachable by id, not listed
  lead(6, { Stage: 'Zen Student Program', Course: 'Data Science', Course_Value: '50000', Payment_Type: 'Direct - Full Payment', Added_Time: zohoTime(hoursAgo(500)) }),
]

const payment = (n, zen, type, amount, verified, addedHoursAgo, verifiedHoursAgo) => ({
  ID: `SEED-P${n}`,
  Zen_ID: zen,
  All_Enrolment: zen,
  Type: type,
  Amount: amount,
  Verified: verified,
  Payment_Type: leads.find((l) => l.zen_id === zen).Payment_Type,
  Mode_Of_Payment: 'Razorpay',
  Payment_Date: zohoDate(hoursAgo(addedHoursAgo)),
  Added_Time: zohoTime(hoursAgo(addedHoursAgo)),
  Verified_on: verifiedHoursAgo == null ? '' : hoursAgo(verifiedHoursAgo).toISOString(),
  'All_Enrolment.Course': leads.find((l) => l.zen_id === zen).Course,
  'All_Enrolment.Course_Value': leads.find((l) => l.zen_id === zen).Course_Value,
  deleted: false,
})

const payments = [
  payment(1, 'SEED-Z1', 'Credit_Booking_Amount', '999.00', '', 58, null),
  payment(2, 'SEED-Z2', 'Credit_Booking_Amount', '999.00', 'Yes', 29, 27),
  payment(3, 'SEED-Z2', 'Credit_Part1', '30000.00', 'Mismatch', 28, null),
  payment(4, 'SEED-Z3', 'Credit_Booking_Amount', '999.00', 'Yes', 7, 6),
  payment(5, 'SEED-Z3', 'Credit_EMI', '17001.00', 'Yes', 6, 5),
  payment(6, 'SEED-Z5', 'Credit_Booking_Amount', '999.00', 'Yes', 39, 38),
  payment(7, 'SEED-Z5', 'Credit_Part1', '72801.00', 'Yes', 38, 10),
]

const emis = [{
  ID: 'SEED-E1', Zen_ID: 'SEED-Z3', EMI_Vendor: 'Loan Partner', EMI_Status: 'Disbursed',
  Loan_amount: '17001.00', st_EMI_Amount: '1700.00', ROI_in: '0.00', Tenor_In_Months: '10',
  Added_Time1: zohoTime(hoursAgo(7)), deleted: false,
}]

const partials = [
  { ID: 'SEED-PR1', Student_ID: 'SEED-L2', Amount: '22000.00', Due_Date: '14-Nov-2026', Actual_Due_Date: '14-Nov-2026', Partial_Status: 'Active', deleted: false },
  { ID: 'SEED-PR2', Student_ID: 'SEED-L2', Amount: '22001.00', Due_Date: '14-Dec-2026', Actual_Due_Date: '14-Dec-2026', Partial_Status: 'Active', deleted: false },
]

const subscriptions = [1, 2, 3, 4].map((n) => ({
  ID: `SEED-SR${n}`, Zen_ID: 'SEED-Z4', Amount: '2250.00', Actual_Amount: '2250.00',
  Due_Date: `01-${MONTHS[(9 + n) % 12]}-${n < 3 ? 2026 : 2027}`, Number_of_Subscription: String(n),
  Payment_Status: 'Link Not Created', deleted: false,
}))
subscriptions.forEach((s) => (s.Actual_Due_Date = s.Due_Date))

const discounts = [{
  ID: 'SEED-D1', Learner_Email_ID: 'learner2@example.com', Status: 'Approved', Actual_Course_Fee: '80000',
  Requested_Course_Fee: '75000', Discount_Value: '5000', Approved_Date_Time: zohoTime(hoursAgo(31)),
  Added_Time: zohoTime(hoursAgo(32)), Requesting_Person: bda2, deleted: false,
}]

const zoho = {
  LeadData: leads,
  paymentData: payments,
  EmiData: emis,
  PartialReminders: partials,
  SubscriptionReminders: subscriptions,
  DiscountData: discounts,
}
for (const [collection, docs] of Object.entries(zoho)) {
  db[collection].deleteMany({ ID: /^SEED-/ })
  db[collection].insertMany(docs)
}

// Feature documents: one open recheck on lead 2 and one old resolved one.
const seededLeads = leads.map((l) => l.ID)
for (const collection of ['salesAuditRechecks', 'salesAuditAlerts', 'salesAuditCcResponses', 'salesAuditAudits']) {
  db[collection].deleteMany({ program: PROGRAM, leadId: { $in: seededLeads } })
}
const created = (at) => ({ at, by: 'seed' })
const mail = (to, subject, sentAt) => ({ to, subject, trigger: 'auto', sentAt })
const raisedOpen = nowSeconds() - 5 * 3600
const raisedOld = nowSeconds() - 200 * 3600
const rechecks = [
  {
    id: newId(), program: PROGRAM, leadId: 'SEED-L2', category: 'payment',
    notes: 'Initial payment on record differs from the agreed 40% split.', status: 'open',
    raisedBy: 'Audit Team', raisedAt: raisedOpen, resolvedAt: null,
    alert: mail(['owner2@example.com', 'managera@example.com'], 'Recheck raised (Payment): Seed Learner 2', raisedOpen),
    created: created(raisedOpen), deleted: false,
  },
  {
    id: newId(), program: PROGRAM, leadId: 'SEED-L1', category: 'downPayment',
    notes: 'Down payment proof was missing at booking.', status: 'resolved',
    raisedBy: 'Audit Team', raisedAt: raisedOld, resolvedAt: raisedOld + 30 * 3600,
    alert: mail(['owner1@example.com', 'managera@example.com'], 'Recheck raised (Down Payment): Seed Learner 1', raisedOld),
    created: created(raisedOld), deleted: false,
  },
]
db.salesAuditRechecks.insertMany(rechecks)
db.salesAuditAlerts.insertMany(rechecks.map((r) => ({
  id: newId(), program: PROGRAM, leadId: r.leadId, kind: 'recheck',
  ...r.alert, delivery: 'skipped', created: r.created, deleted: false,
})))

print(`Seeded ${leads.length} leads, ${payments.length} payments and ${rechecks.length} rechecks.`)
