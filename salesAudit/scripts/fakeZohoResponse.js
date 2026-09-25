// Prints a fake Zoho learner API response (same shape as the real one, no real PII) for
// POST /sales-audit/zoho/import. The BDAs/BDMs match salesAudit/scripts/seed.js.
//   node salesAudit/scripts/fakeZohoResponse.js [count=100] > zoho.json
const count = Number(process.argv[2]) || 100

const pad = (n, size = 2) => String(n).padStart(size, '0')
const day = (offset) => {
  const date = new Date(Date.now() - offset * 86400000)
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}
const pick = (list, n) => list[n % list.length]

const products = ['Zen_Full_Stack_Development', 'Zen_Data_Science', 'Zen_Business_Analytics_Digital_Marketing']
const plans = ['Direct - Partial Payment', 'Direct - Full Payment', 'EMI - 12 Month', 'Subscription']
const firstNames = ['Asha', 'Ravi', 'Meena', 'Karan', 'Divya', 'Arjun', 'Priya', 'Vikram', 'Nila', 'Rohit']

const learners = []
for (let n = 0; n < count; n++) {
  // 1 in 5 leads is North, the rest South
  const north = n % 5 === 0
  const team = north ? 'North' : 'South'
  const bda = north ? pick(['bda.north1', 'bda.north2'], n) : pick(['bda.south1', 'bda.south2'], n)
  const plan = pick(plans, n)
  const fee = pick([70000, 90999, 120000], n)
  const enrolled = day(n % 20)
  const records = [{ recordId: 800000 + n * 3, type: 'Credit_Booking_Amount', amount: 999, verified: 'Yes',
    zbModeOfPayment: 'Razorpay', utrPaymentId: `pay_fake${n}a`, paymentDate: enrolled, verifiedDate: enrolled }]
  const second = plan === 'Direct - Full Payment' ? fee - 999 : plan === 'EMI - 12 Month' ? fee * 0.4 : 20000
  records.push({ recordId: 800001 + n * 3, type: 'Credit_Part1', amount: second,
    // 1 in 6 leads still has an unverified payment (Sales Action Pending)
    verified: n % 6 === 1 ? '' : 'Yes', zbModeOfPayment: 'Razorpay', utrPaymentId: `pay_fake${n}b`, paymentDate: enrolled })
  learners.push({
    zenId: String(700000 + n),
    superleapId: `FAKE_${pad(n, 5)}`,
    name: `${pick(firstNames, n)} Learner${n}`,
    email: `learner${n}@example.com`,
    phone: `+9100000${pad(n, 5)}`,
    preferredLanguage: pick(['English', 'Tamil', 'Hindi'], n),
    saleOwner: `${bda}@example.com`,
    saleOwnerManager: north ? 'bdm.north@example.com' : 'bdm.south@example.com',
    salesTeam: team,
    salesFrom: `${team} Mainboot`,
    Stage: 'Main Boot ATTENDING',
    status: 'converted',
    product: pick(products, n),
    modeOfStudy: pick(['WeekDAY', 'WeekEND'], n),
    dateOfEnrollment: enrolled,
    crmLeadCreatedDate: `${enrolled} 10:00:00`,
    paymenttype: plan,
    partialCategory: plan === 'Direct - Partial Payment' ? '50-50' : '',
    courseFee: fee,
    totalPaid: records.reduce((sum, r) => sum + r.amount, 0),
    balaceAmount: 0,
    'terms&conditions': n % 9 === 0 ? 'No' : 'Yes',
    // 1 in 4 leads is still waiting for its CC from Superleap
    confirmationCall: n % 4 === 3 ? '' : `https://drive.google.com/file/d/FAKEFILE${n}/view`,
    batchData: { batchName: `B${pad(n % 12)}`, batchType: 'WeekDAY', language: 'English', startDate: day(-10), endDate: day(-200), startTime: '18:00:00', status: 'Active' },
    admissionDetails: { highestDegree: 'B.E', yearOfPassing: '2022', gender: pick(['Male', 'Female'], n), workExp: '1 year', '12thPercentage': 80 },
    financialDetails: records,
    EMIdetails: plan.startsWith('EMI')
      ? [{ recordId: 900000 + n, applicationId: `APP${n}`, emiVendor: 'FakeFinance', emiStatus: 'Approved', stage: 'Disbursed',
          loanAmount: fee * 0.6, disbursalAmount: fee * 0.6, firstEMIamount: Math.round((fee * 0.6) / 12), tenorInMonth: '12', ROIinPercentage: '0', applicationDate: enrolled }]
      : [],
    CourseDiscountDetails: [{ requestedCourseFee: fee, actualCourseFee: fee + 10000, discountValue: 10000, requestedperson: 'bdm.south@example.com', paymentType: plan, status: 'Approved' }],
    partialReminders: [],
    subscriptionReminders: [],
    recheckDetails: [],
  })
}
process.stdout.write(JSON.stringify({ code: 3000, result: learners }))
