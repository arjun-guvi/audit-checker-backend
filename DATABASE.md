# Sales Audit: database structure and data flow

This explains how the Sales Audit feature stores its data in MongoDB, and which documents each step of the audit flow reads and writes. The field lists come from `salesAudit/models/documents.go`, and the indexes from `salesAudit/scripts/indexes.js`. If this file and the code ever disagree, the code is right.

---

## Contents

1. [The basics](#1-the-basics)
2. [How the collections relate](#2-how-the-collections-relate)
3. [Collections](#3-collections)
4. [The flow, step by step](#4-the-flow-step-by-step)
5. [The lead's audit status](#5-the-leads-audit-status)
6. [Who owns which lead fields](#6-who-owns-which-lead-fields)
7. [Zoho data and how it maps to ours](#7-zoho-data-and-how-it-maps-to-ours)
8. [How the code reaches the database](#8-how-the-code-reaches-the-database)
9. [Looking at the data yourself](#9-looking-at-the-data-yourself)

---

## 1. The basics

- **Database:** set by `MONGO_DATABASE` (default `audit_app`). `audit_app` holds mock data and `audit_live` holds real Zoho data; both have the same structure. The feature owns nine collections, all prefixed `salesAudit`.
- **IDs:** every document has its own `id`, a UUID string. The code never uses Mongo's `_id`, and documents point at each other through `id` (for example, `recheck.leadId` holds `lead.id`).
- **Program:** every document has a `program` (the tenant, `SALES_AUDIT_PROGRAM`, default `guvi`). Every query filters on it.
- **Soft delete:** every document has `deleted`. Queries only return `deleted: false`, and nothing is removed physically.
- **Created:** every document has `created: { at, by }`. `by` is the user hash of whoever caused it, or `system` for the Zoho import and scheduled jobs.
- **Time:** every timestamp is **Unix seconds** (not milliseconds). Some values copied straight from Zoho (such as `course.enrolledOn` or a payment's `paymentDate`) stay as the date strings Zoho sent.
- **Emails:** stored lower-case. Members, auditors, BDAs and BDMs are identified by email across collections.

## 2. How the collections relate

```
                         salesAuditMembers
                  (email, role, region, managerEmail)
                     ▲             ▲            ▲
       assignment.auditorEmail  bdaEmail    bdmEmail          recipientEmail
                     │             │            │                   │
┌────────────────────┴─────────────┴────────────┴──┐     ┌──────────┴──────────────┐
│                 salesAuditLeads                  │     │ salesAuditNotifications │
│   one per Zoho enrolment (zenId is unique)       │     │  in-app alerts          │
│   + workflow state: assignment, audit,           │     └─────────────────────────┘
│     recheckSummary                               │
└──▲──────────────▲──────────────▲─────────────▲───┘
   │ leadId       │ leadId       │ leadId      │ leadId (unique)
┌──┴───────────┐ ┌┴────────────┐ ┌┴──────────┐ ┌┴─────────────────────┐
│ salesAudit   │ │ salesAudit  │ │ salesAudit│ │ salesAuditCcExtracts │
│ Audits       │ │ Rechecks    │ │ Events    │ │ what was read from   │
│ one per      │◄┤ tickets     │ │ timeline  │ │ the CC PDF / call    │
│ attempt      │ │ auditId ──► │ └───────────┘ └──────────────────────┘
│ recheckId ──►│ │             │
└──────────────┘ └─────────────┘   salesAuditAlerts    mail log (leadId, may be empty)
                                   salesAuditCounters  sequences (recheck numbers)
```

- **Members** hold the org chart. An auditor's `managerEmail` is the auditor TL; a BDA's `managerEmail` is their BDM.
- **Lead** is the centre. Everything else points at it through `leadId`.
- An **audit** attempt that ends in a recheck points at it (`audit.recheckId`), and the recheck points back (`recheck.auditId`).
- The lead keeps a small **copy** of its rechecks (`recheckSummary`), so lists can show "1 open / 3" without reading the rechecks collection.

## 3. Collections

### `salesAuditMembers`: who uses the feature

| Field | Meaning |
|---|---|
| `userHash` | Zen's user hash; this is what the `Authorization` token is matched against |
| `email` | Unique per program. In dev, the token `dev-mock-token:<email>` signs in as this member |
| `name` | Display name |
| `role` | `auditorTl`, `auditor`, `bdm` or `bda` |
| `region` | `North` or `South`, for auditors (assignment goes by region) |
| `managerEmail` | Auditor → TL, BDA → BDM. This is how "my team" is worked out |
| `available` | `false` takes an auditor out of auto-assignment |

Indexes: `{program, email}` unique, `{program, userHash}`.

### `salesAuditLeads`: one per Zoho enrolment

The biggest document. It has two halves: data copied from Zoho, and the portal's workflow state (see [section 6](#6-who-owns-which-lead-fields)).

| Field | Meaning |
|---|---|
| `zenId` | Zoho's learner ID. Unique per program; the import finds existing leads by it |
| `superleapId`, `salesFrom`, `stage`, `zohoStatus` | Zoho / CRM references |
| `region` | `North` / `South`, from Zoho's sales team. Decides which auditors can get the lead |
| `personal` | `{name, email, phone, preferredLanguage}` |
| `course` | `{product, modeOfStudy, enrolledOn, onboardingAt, batch{name, type, language, startDate, endDate, startTime, status}}` |
| `payment` | `{paymentType, partialCategory, courseFee, totalPaid, balanceAmount, promoCode, …}`, plus the arrays `records` (Zoho financial records, each with `verified`), `emis`, `discounts`, `partialReminders`, `subscriptions` |
| `payment.ready`, `payment.shortfall`, `payment.verifiedAmount` | Worked out on import: `ready` is true once every payment is verified and the plan's minimum is met; `shortfall` says why not |
| `admission` | The admission form, as `{label: value}` |
| `termsAccepted` | T&C accepted |
| `marketing` | `{source, medium, campaign, content, affiliateId}` |
| `bdaEmail`, `bdmEmail` | The sales owner and their manager, lower-case |
| `onboardCoordinator` | Zoho's onboarding coordinator, lower-case email |
| `cc` | The confirmation call: `{link, type (pdf / recording / link), status (pending / updated), updatedAt}` |
| `assignment` | `null` until assigned; then `{auditorEmail, assignedAt, assignedBy, mode (auto / manual / takeUp / zoho)}` |
| `audit` | `{status, attempt, lastAuditedAt, completedAt, completedBy}`. See [section 5](#5-the-leads-audit-status) |
| `recheckSummary` | `{open, total, lastRaisedAt, lastClosedAt}`, recomputed from the rechecks |
| `crmCreatedAt`, `enrolledAt`, `zohoSyncedAt` | When the lead was created in the CRM, enrolled, and last imported |
| `paymentVerificationMailedAt`, `lastEscalationAt` | When the payment mails last went out (0 = never). They stop the sweeps from repeating themselves |

Indexes: `{program, zenId}` unique, plus one index per list filter: auditor + status, BDA, BDM, region + status, CC status, completed date, enrolled date.

A trimmed example, from the demo data:

```js
{
  id: '0e0e41f9-…', program: 'guvi', zenId: '700020', region: 'North',
  personal: { name: 'Asha Learner20', email: 'learner20@example.com', phone: '+910000000020' },
  course: { product: 'Zen_Business_Analytics_Digital_Marketing', batch: { name: 'B08', startDate: '2026-10-05' } },
  payment: {
    paymentType: 'Direct - Partial Payment', courseFee: 120000, totalPaid: 20999,
    ready: true, shortfall: '', verifiedAmount: 20999,
    records: [ { type: 'Credit_Booking_Amount', amount: 999, verified: 'Yes' }, { type: 'Credit_Part1', amount: 20000, verified: 'Yes' } ]
  },
  bdaEmail: 'bda.north1@example.com', bdmEmail: 'bdm.north@example.com',
  cc: { link: 'https://drive.google.com/file/d/…/view', type: 'pdf', status: 'updated', updatedAt: 1790312934 },
  assignment: { auditorEmail: 'auditor.north@example.com', assignedBy: 'tl@example.com', mode: 'auto' },
  audit: { status: 'recheckOpen', attempt: 1, lastAuditedAt: 1790312945, completedAt: 0 },
  recheckSummary: { open: 1, total: 1, lastRaisedAt: 1790312945, lastClosedAt: 0 },
  created: { at: 1790312934, by: 'system' }, deleted: false
}
```

### `salesAuditAudits`: one per audit attempt

A lead can be audited several times (after each recheck is fixed), so there can be many per lead.

| Field | Meaning |
|---|---|
| `leadId` | The lead |
| `attempt` | 1, 2, 3… |
| `auditor` | `{email, name, role}` of whoever submitted it |
| `outcome` | `completed` (checklist submitted) or `recheckRaised` |
| `checklist` | `[{key, label, checked}]`. Empty when the outcome is a recheck |
| `comments` | The auditor's comments |
| `mismatchCount` | How many fields differed between our record and the CC at that moment |
| `recheckId` | The recheck this attempt raised, if any |
| `submittedAt` | When |

Indexes: `{leadId, submittedAt}` (not unique, since there are many attempts per lead), `{auditor.email, submittedAt}`.

### `salesAuditRechecks`: recheck tickets

| Field | Meaning |
|---|---|
| `recheckNo` | Unique. `RC-000123` for portal rechecks (from the counter), or Zoho's SRID for imported ones |
| `source` | `portal` or `zoho` |
| `leadId`, `leadName`, `zenId` | The lead. Name and Zen ID are copied in so ticket lists need no join |
| `auditId`, `attempt` | The audit attempt that raised it |
| `reasons` | `[{category, comments}]`: everything the BDA must fix, one category each (no category twice). Categories are `ccPending`, `payment`, `emi`, `approval`, `missedPointsInCc`, `downPayment` |
| `category` | The first reason's category. Kept so older rechecks and Zoho's (which have no `reasons`) read the same way. Code reads reasons through `core.ReasonsOf` |
| `comments` | Every reason in one line: just the comments for one reason, `Payment: …; EMI: …` for several |
| `status` | `open` or `closed` |
| `raisedBy`, `raisedAt` | Who raised it and when |
| `bdaEmail`, `bdmEmail`, `auditorEmail` | Copied from the lead when raised. They decide who sees the ticket and who is mailed |
| `closed` | `null` while open; then `{at, by{email, name, role}, note}`. The note says what was fixed |
| `reauditedAt` | 0 until the lead is audited again after the close. "Closed · audit pending" means `status: closed` and `reauditedAt: 0` |
| `alert`, `lastReminder` | The mail sent when it was raised, and the latest reminder mail: `{to, subject, trigger, sentAt}` |
| `ccUpdatedAt` | Set on an open **CC recheck** (a *CC Pending* or *Missed points in CC* reason) when the import sees the lead's CC updated after the recheck was raised: the fix is in, but the ticket is still open. 0 otherwise |
| `ccCloseAlert` | The last "CC updated, close the ticket" mail to the BDA and BDM |

Filtering by category matches a recheck when **any** of its reasons has that category, and the BDA dashboard counts each reason under its own category.

Indexes: `{recheckNo}` unique, `{leadId, raisedAt}`, `{status, closed.at}`, and `{bdaEmail | bdmEmail | auditorEmail, status}` for each role's ticket list.

### `salesAuditEvents`: the lead timeline

Written once and never changed. The lead page's timeline is these events, oldest first.

| Field | Meaning |
|---|---|
| `leadId` | The lead |
| `type` | `leadImported`, `assigned`, `reassigned`, `takenUp`, `ccUpdated`, `auditCompleted`, `recheckRaised`, `recheckClosed` |
| `actor` | `{email, name, role}`; role `system` for the import |
| `at` | When it happened (for `leadImported`, when the learner enrolled) |
| `data` | Depends on the type, e.g. `{auditorEmail, mode, region}` for `assigned`, `{recheckNo, category, comments, attempt}` for `recheckRaised` |

Index: `{leadId, at}`.

### `salesAuditNotifications`: in-app alerts (the bell)

One document per recipient.

| Field | Meaning |
|---|---|
| `recipientEmail` | Who sees it |
| `type` | `leadsAssigned`, `leadReassigned`, `recheckRaised`, `recheckClosed`, `ccUpdated` |
| `leadId`, `recheckId` | What it is about (empty for a digest such as "40 new leads") |
| `title`, `message` | The text shown |
| `read`, `readAt` | Read state |

Index: `{recipientEmail, read, created.at}`.

### `salesAuditAlerts`: mail log

Every mail is written here **before** it is queued, so there is a record even when sending fails.

| Field | Meaning |
|---|---|
| `leadId` | The lead, or empty for digests |
| `kind` | `recheck`, `recheckReminder`, `recheckClosed`, `leadsAssigned`, `ccUpdated`, `escalation`, `paymentVerificationPending` |
| `to`, `subject`, `body` | The mail |
| `trigger` | `auto` or `manual` (e.g. the "Send reminder" button) |
| `sentAt` | When it was created |
| `delivery` | `queued` → `sent`, `failed`, or `skipped` (no SMTP configured) |

Indexes: `{leadId, kind, sentAt}`, `{sentAt}`.

### `salesAuditCcExtracts`: what was read from the CC

One per lead. It is cached until the lead's CC link changes.

| Field | Meaning |
|---|---|
| `leadId` | Unique |
| `link`, `type` | The CC it was read from. When `lead.cc.link` changes, it is read again |
| `fields` | `[{key, value}]`: name, email, product, batch, fee, discount… These are what the audit page compares against the lead |
| `transcript` | `[{speaker, at, text}]` for call recordings |
| `pointsCovered` | The mandatory points the call covered |
| `mocked` | `true` while the CC reader is a mock (until the transcription service exists) |

### `salesAuditCounters`: sequences

`{name, value}`. For now there is only `recheckNo`. It is increased atomically (`findOneAndUpdate` with `$inc` and upsert), so two auditors can't get the same `RC-` number.

## 4. The flow, step by step

What each step writes. **R** = read, **I** = insert, **U** = update.

### 1. Leads come in (Zoho import)

The worker runs it every 15 minutes, or the TL runs it with *Sync from Zoho* / *Import Zoho file* (`POST /zoho/import`).

| Collection | What happens |
|---|---|
| `salesAuditLeads` | **R** by `zenId`. New → **I**, with `audit.status: unassigned` and `assignment: null`. Existing → **U** of the Zoho fields only |
| `salesAuditEvents` | **I** `leadImported`; **I** `ccUpdated` when a CC link has appeared; **I** `recheckRaised` / `recheckClosed` for Zoho's rechecks |
| `salesAuditRechecks` | **I** each recheck Zoho has that isn't stored yet (`source: zoho`, `recheckNo` = the SRID). **U** a stored Zoho recheck to `closed` once Zoho shows it closed |
| `salesAuditNotifications`, `salesAuditAlerts` | **I** "CC can be verified" to the auditor when a CC arrives on an assigned lead |

Then assignment runs straight away (next step).

### 2. Leads are assigned

This is automatic after the import, or the TL uses *Assign unassigned leads* (`POST /leads/assign`).

| Collection | What happens |
|---|---|
| `salesAuditMembers` | **R** the available auditors, grouped by `region` |
| `salesAuditLeads` | **R** the leads with `assignment: null`; each goes to the least-loaded auditor of its region (ties random). **U** `assignment`, and `audit.status` becomes `pending` |
| `salesAuditEvents` | **I** `assigned` per lead |
| `salesAuditNotifications`, `salesAuditAlerts` | **I** one digest per auditor ("40 new leads assigned to you") |

*Reassign* (TL) and *Take up* (auditor) work the same way on one lead: `mode` is `manual` / `takeUp`, the event is `reassigned` / `takenUp`, and the lead's rechecks that are open or still waiting for a re-audit get the new `auditorEmail`.

### 3. The auditor opens the audit

Pages: `GET /leads/:id/audit` and `/cc-verification`.

| Collection | What happens |
|---|---|
| `salesAuditLeads` | **R** the lead |
| `salesAuditCcExtracts` | **R** the extract; if it is missing or the CC link changed, the CC is read again and the extract is **I/U** |
| `salesAuditRechecks` | **R** the lead's rechecks |

Nothing about the lead changes. The side-by-side comparison is worked out on every request from the lead and the extract.

### 4a. The auditor completes the audit (checklist)

`POST /leads/:id/complete-audit`. This is only allowed when every checklist item is ticked, there is a comment, the payment is `ready`, and no recheck is open.

| Collection | What happens |
|---|---|
| `salesAuditAudits` | **I** an attempt with `outcome: completed` and the checklist |
| `salesAuditRechecks` | **U** `reauditedAt` on closed rechecks that were waiting for this re-audit |
| `salesAuditLeads` | **U** `audit` → `{status: completed, attempt, completedAt, completedBy}` and `recheckSummary` |
| `salesAuditEvents` | **I** `auditCompleted` |

### 4b. The auditor raises a recheck instead

`POST /rechecks {leadId, reasons: [{category, comments}, …]}`

| Collection | What happens |
|---|---|
| `salesAuditCounters` | **U** `recheckNo` + 1 → `RC-000124` |
| `salesAuditAlerts` | **I** the "Recheck raised" mail to the BDA and BDM (then queued) |
| `salesAuditRechecks` | **I** one ticket with every reason, `status: open` |
| `salesAuditAudits` | **I** an attempt with `outcome: recheckRaised`, linked both ways to the recheck |
| `salesAuditLeads` | **U** `audit.status: recheckOpen`, `attempt + 1`, and `recheckSummary` (`open + 1`) |
| `salesAuditEvents` | **I** `recheckRaised` |
| `salesAuditNotifications` | **I** one each for the BDA and the BDM |

### 5. The BDA (or BDM or an auditor) closes the ticket

`POST /rechecks/:id/close {note}`

| Collection | What happens |
|---|---|
| `salesAuditRechecks` | **U** `status: closed`, `closed: {at, by, note}` |
| `salesAuditLeads` | **U** `recheckSummary`. When no recheck is still open, `audit.status` goes from `recheckOpen` to `recheckClosed` ("audit again") |
| `salesAuditEvents` | **I** `recheckClosed`, with who closed it |
| `salesAuditNotifications`, `salesAuditAlerts` | **I** "Audit again" to the lead's auditor |

#### CC updated, ticket left open

If the ticket is a CC recheck and the BDA updates the CC in Zoho without closing the ticket, the next import (for a lead it already knew) does this:

| Collection | What happens |
|---|---|
| `salesAuditRechecks` | **U** `ccUpdatedAt` = the lead's `cc.updatedAt`, and `ccCloseAlert` |
| `salesAuditNotifications`, `salesAuditAlerts` | **I** "Close recheck RC-…: CC updated … ago" to the BDA and BDM |

The hourly recheck sweep repeats that alert every 24h, with the time since the CC update, in place of the ordinary reminder, until the ticket is closed. `GET /rechecks?view=ccUpdatedNotClosed` lists these tickets.

The auditor then goes back to step 3. The loop repeats until step 4a completes the audit.

### Background sweeps

The worker runs these on Redis (see README section 6).

| Job | Reads | Writes |
|---|---|---|
| Escalation (hourly) | leads with an unverified payment for over 24h | `salesAuditAlerts` (mail to BDA, BDM, Accounts); lead `lastEscalationAt` |
| Payment verification (every 10 min) | same | `salesAuditAlerts` (mail to the BDM, once); lead `paymentVerificationMailedAt` |
| Recheck reminder (hourly) | rechecks open for over 24h | `salesAuditAlerts`; recheck `lastReminder`. For a CC recheck whose CC is updated: the CC close alert instead, recheck `ccCloseAlert` |
| Send mail (queued) | `salesAuditAlerts` by id | `delivery` → `sent` / `failed` / `skipped` |

## 5. The lead's audit status

`lead.audit.status` is stored, but after every recheck change it is recomputed from the lead's rechecks (`core.ReconcileStatus` in `salesAudit/core/workflow.go`):

```
unassigned ──assign──► pending ──complete──────────────────────────────► completed
                          │                                                  ▲
                          └──raise recheck──► recheckOpen ──close all──► recheckClosed
                                                  ▲                          │
                                                  └───── raise another ──────┤
                                                                             └──complete──┘
```

Rules, in order:
1. `completed` stays `completed`. Completing is refused while a recheck is open, and a recheck can't be raised on a completed lead.
2. Any recheck still open → `recheckOpen`.
3. Was `recheckOpen`, and now none is open → `recheckClosed`.
4. Not assigned → `unassigned`.
5. Assigned but `unassigned` (or empty) → `pending`.

`recheckSummary` is always rebuilt by counting the lead's rechecks (`core.SummarizeRechecks`), never increased or decreased by hand. It can't drift.

## 6. Who owns which lead fields

The import and the portal write different parts of a lead, so neither overwrites the other:

| Written by | Fields | Store function |
|---|---|---|
| **Zoho import** (every sync) | `superleapId`, `region`, `salesFrom`, `stage`, `zohoStatus`, `personal`, `course`, `payment`, `admission`, `termsAccepted`, `marketing`, `bdaEmail`, `bdmEmail`, `cc`, `zohoSyncedAt`, … | `UpdateLeadZoho` |
| **Portal workflow** | `assignment`, `audit`, `recheckSummary` | `UpdateLeadWorkflow` |
| **Sweeps** | `paymentVerificationMailedAt`, `lastEscalationAt` | `SetLeadMailMarks` |

Zoho's own `auditCoordinator`, `auditStatus` and `recheckDetails` are only used **once**, when a lead is first imported, to set its starting state. After that the portal owns them, and nothing is written back to Zoho.

## 7. Zoho data and how it maps to ours

The Zoho record shape is `models.ZohoLearner` (`salesAudit/models/zoho.go`), and the mapping is in `salesAudit/core/zohomap.go`. If this section and those files disagree, the code is right.

### Where the data comes from

- **API:** `GET ZOHO_API_URL?publickey=ZOHO_API_PUBLIC_KEY&from=…&to=…` (`salesAudit/zoho/client.go`). The window is `ZOHO_API_FROM` / `ZOHO_API_TO` when both are set, otherwise the last `ZOHO_SYNC_LOOKBACK_DAYS` days (default 3) up to today, as IST dates in `DD-Mon-YYYY`.
- **File:** `POST /zoho/import` with a body imports that JSON instead of calling Zoho. It must have the same shape the API returns.
- **Backfill:** `POST /zoho/backfill {from, to}` calls the API once per 5-day window of the range (`core.ZohoChunks`), oldest first. Each window runs the same import as below, also adds BDA/BDM members (see [below](#bdas-and-bdms--salesauditmembers)), and sends no mails or notifications. This is how `audit_live` is filled (README, *Live data*).
- **Response shape:** `{"result": [...]}`, `{"data": [...]}`, a bare list, or a single learner are all accepted. A record that can't be decoded is skipped (and logged) rather than failing the batch, and a learner without a `zenId` is counted as skipped.

### How values are read

Zoho is loose with types, so every scalar is read as text first (`models.ZohoValue`), then converted:

| Kind | Rule |
|---|---|
| Text | Trimmed. `null`, objects and arrays become `""`; numbers and booleans are kept as written |
| Amounts | `core.Amount`: `,` and a leading `₹` are dropped, then parsed as a number. Empty or unreadable → `0` |
| Emails | `core.NormalizeEmail`: Zoho often sends `Name - email`, so only the part after the last ` - ` is kept, lower-cased |
| Dates → Unix seconds | `core.ZohoSeconds`: RFC 3339, or one of `2006-01-02 15:04:05(.0)`, `2006-01-02`, `02-Jan-2006( 15:04:05)`, `01/02/06(06) 03:04:05 PM`, read as IST. Empty or unreadable → `0` |
| Dates kept as text | Batch dates, payment / EMI / reminder dates and `course.enrolledOn` are stored exactly as Zoho sent them |

### Learner → `salesAuditLeads`

These are refreshed on **every** import (`core.LeadFromZoho`).

| Zoho field | Lead field | Notes |
|---|---|---|
| `zenId` | `zenId` | The match key. Unique per program |
| `superleapId` | `superleapId` | |
| `salesTeam` | `region` | `north` → `North`, `south` → `South` (any case); anything else → `""`, and the lead can't be auto-assigned |
| `salesFrom` | `salesFrom` | |
| `Stage` | `stage` | Capital `S` in Zoho |
| `status` | `zohoStatus` | |
| `name` | `personal.name` | Repeated spaces collapsed |
| `email` | `personal.email` | Lower-cased |
| `phone` | `personal.phone` | Kept as text, even when Zoho sends a number |
| `preferredLanguage` | `personal.preferredLanguage` | |
| `product` | `course.product` | |
| `modeOfStudy` | `course.modeOfStudy` | |
| `dateOfEnrollment` | `course.enrolledOn` (text) **and** `enrolledAt` (seconds) | |
| `onboardingDateTime` | `course.onboardingAt` | Seconds |
| `batchData.{batchName, batchType, language, startDate, endDate, startTime, status}` | `course.batch.{name, type, language, startDate, endDate, startTime, status}` | Text |
| `crmLeadCreatedDate` | `crmCreatedAt` | Seconds |
| `saleOwner` | `bdaEmail` | Normalised email |
| `saleOwnerManager` | `bdmEmail` | Normalised email |
| `onboardCoordinator` | `onboardCoordinator` | Normalised email |
| `admissionDetails` | `admission` | Every key kept as is, values as text |
| `terms&conditions` | `termsAccepted` | `true` only for `Yes` (any case) |
| `source`, `medium`, `campaign`, `content`, `affiliateId` | `marketing.{source, medium, campaign, content, affiliateId}` | |
| `confirmationCall` | `cc.link` | Also sets `cc.type` and `cc.status`, see below |
| `paymenttype` | `payment.paymentType` | Lower-case `t` in Zoho |
| `partialCategory` | `payment.partialCategory` | |
| `courseFee` | `payment.courseFee` | Amount |
| `totalPaid` | `payment.totalPaid` | Amount |
| `balaceAmount` | `payment.balanceAmount` | Zoho's spelling (`balace`); amount |
| `promoCode` | `payment.promoCode` | |
| `willLeadPayinSameMonth` | `payment.payInSameMonth` | Text |
| `zbCustomerId`, `zbInvoiceId` | `payment.zbCustomerId`, `payment.zbInvoiceId` | Zoho Books references |
| *(worked out)* | `payment.ready`, `payment.shortfall`, `payment.verifiedAmount` | `core.WithPaymentReadiness`, from the records below |
| *(import time)* | `zohoSyncedAt` | When this import ran |

**The CC.** `cc.status` is `updated` when `confirmationCall` has a link and `pending` when it is empty. `cc.type` comes from the link (`core.CcTypeOf`): Superleap links, links with `recording` in them, and `.mp3` / `.wav` / `.m4a` / `.ogg` / `.aac` files are `recording`; Drive file links (`drive.google.com/file/d/…`) and `.pdf` files are `pdf`; anything else (Drive folders, Gmail threads) is `link`. `cc.updatedAt` is set to the import time when the CC first appears or its link changes, and kept otherwise.

### Payment arrays

Each Zoho array becomes an array under `payment`. Amounts are converted to numbers; everything else stays text.

| Zoho array → lead field | Zoho field → our field |
|---|---|
| `financialDetails` → `payment.records` | `recordId` → `recordId`, `type` → `type`, `amount` → `amount`, `zbModeOfPayment` → `modeOfPayment`, `utrPaymentId` → `utrPaymentId`, `verified` → `verified`, `verifiedDate` → `verifiedDate`, `paymentDate` → `paymentDate`, `zbReceiptCreated` → `receiptMade` |
| `EMIdetails` → `payment.emis` | `recordId` → `recordId`, `applicationId` → `applicationId`, `emiVendor` → `vendor`, `emiStatus` → `status`, `stage` → `stage`, `loanAmount` → `loanAmount`, `disbursalAmount` → `disbursalAmount`, `firstEMIamount` → `firstEmiAmount`, `tenorInMonth` → `tenorMonths`, `ROIinPercentage` → `roiPercent`, `applicationDate` → `applicationDate`, `applicationInTheNameOf` → `inTheNameOf` |
| `CourseDiscountDetails` → `payment.discounts` | `requestedCourseFee` → `requestedCourseFee`, `actualCourseFee` → `actualCourseFee`, `discountValue` → `discountValue`, `requestedperson` → `requestedBy` (normalised email), `paymentType` → `paymentType`, `status` → `status` |
| `partialReminders` → `payment.partialReminders` | `recordId` → `recordId`, `noOfPartial` → `noOfPartial`, `amount` → `amount`, `dueDate` → `dueDate`, `partialpaymentStatus` → `status`, `linkStatus` → `linkStatus`, `paidDateTime` → `paidAt` |
| `subscriptionReminders` → `payment.subscriptions` | `recordId` → `recordId`, `noOfSubscription` → `noOfSubscription`, `amount` → `amount`, `dueDate` → `dueDate`, `paymentStatus` → `status` |

A missing array is stored as `[]`, never `null`.

### Starting workflow state (first import only)

When a `zenId` is new, `core.NewLeadFromZoho` also uses these fields to set the lead's starting state. On later imports they are **ignored**, because the portal owns the workflow ([section 6](#6-who-owns-which-lead-fields)).

| Zoho field | Lead field | Rule |
|---|---|---|
| `auditCoordinator` | `assignment.auditorEmail` | Normalised email. When set, the lead starts assigned with `mode: zoho` and `assignedBy: system`; when empty, `assignment` is `null` |
| `auditCoordinatorAssignedDateTime` | `assignment.assignedAt` | Seconds; the import time when missing |
| `auditStatus` | `audit.status` | `Completed` → `completed`, `Recheck Pending` → `recheckOpen`, otherwise `pending` if assigned, `unassigned` if not |
| `auditCompletedDateTime` | `audit.completedAt`, `audit.lastAuditedAt` | Only for `completed`; the import time when missing. `completedBy` is the coordinator and `attempt` is `1` |

It also writes the lead's opening events: `leadImported` (dated at `enrolledAt`, falling back to `crmCreatedAt`, then now), plus `assigned`, `ccUpdated` and `auditCompleted` where they apply.

### `recheckDetails` → `salesAuditRechecks`

Every import walks the learner's `recheckDetails` (`core.RecheckFromZoho`). An entry without an `SRID` is skipped.

| Zoho field | Recheck field | Notes |
|---|---|---|
| `SRID` | `recheckNo` | The match key: a recheck with this number that isn't stored yet is inserted, with `source: zoho` |
| `pendingList` | `category`, `reasons[0].category` | The first entry that is recognised: `Confirmation Call` / `CC Pending` → `ccPending`, `Missed points on CC` / `Missed points in CC` → `missedPointsInCc`, `Payment` → `payment`, `EMI` → `emi`, `Approval` → `approval`, `Down Payment` → `downPayment`. None recognised → `payment` |
| `auditComments` | `comments`, `reasons[0].comments` | A Zoho recheck always has exactly one reason |
| `requestPerson` | `raisedBy` | Normalised email, role `auditor` |
| `recheckDate` | `raisedAt` | Seconds; the import time when missing |
| `recheckattempt` | `attempt` | Number; `0` when unreadable |
| `ticketStatus` | `status` | `closed` (any case) → `closed`, anything else → `open` |
| *(from the lead)* | `leadId`, `leadName`, `zenId`, `bdaEmail`, `bdmEmail`, `auditorEmail` | Copied from the lead as it is after this import |

A Zoho recheck that is **already closed** when first seen gets `closed: {at: raisedAt, by: system "Zoho", note: "Closed in Zoho before the portal took over"}` and `reauditedAt: raisedAt`, so old Zoho history doesn't show up as "Closed · audit pending".

A stored Zoho recheck that is still `open` is closed once Zoho shows `ticketStatus: closed`, with `closed: {at: now, by: system, note: "Closed in Zoho"}`. Nothing else on a stored recheck is updated from Zoho, and portal rechecks (`source: portal`) are never touched by the import.

### BDAs and BDMs → `salesAuditMembers`

Only the backfill does this; the regular import and the 15-minute sync don't (`actions.SyncSalesMembers`).

| Zoho field | Member | Rule |
|---|---|---|
| `saleOwnerManager` | a `bdm` member | Added when no member has this email |
| `saleOwner` | a `bda` member, `managerEmail` = `saleOwnerManager` | Added when missing. An existing `bda` whose `managerEmail` differs is moved to the BDM Zoho names now (windows run oldest first, so the latest wins) |

Emails are normalised as on the lead, so `member.email` always equals `lead.bdaEmail` / `lead.bdmEmail`. An existing member with a different role (for example an auditor) is never changed. Members added here have `name` = email, `userHash: ""` (no sign-in until the TL sets it) and `created.by: system`.

This is what the BDM → BDA → leads tree rests on:
- a BDM's BDAs: members with `role: 'bda'` and `managerEmail` = the BDM (`core.TeamEmails`);
- a BDA's leads: leads with `bdaEmail` = the BDA;
- a BDM's leads: leads with `bdmEmail` = the BDM **or** `bdaEmail` in their BDAs (`core.ScopeFor`), i.e. every lead of every BDA under them.

### Zoho fields we don't store

`auditStatus`, `auditCoordinator`, `auditCoordinatorAssignedDateTime` and `auditCompletedDateTime` are used only on the first import, as above. Anything in a Zoho record that isn't listed in `models.ZohoLearner` is dropped when it is decoded; to keep a new Zoho field, add it there and map it in `core/zohomap.go`.

## 8. How the code reaches the database

```
controllers/ (HTTP) ──► actions/ (use cases) ──► store.X(...)  ──► store/mongo.go (Mongo)
                              │                        │
                              ▼                        └─ in tests: store/fake (in memory)
                         core/ (pure rules: status, payment, scope, matching)
```

- `salesAudit/store/store.go` declares every database call as a **function variable**, e.g. `store.FindLeads = findLeads`.
- `salesAudit/store/mongo.go` has the real versions. List filters are built in `leadFilter` and `recheckFilter`.
- `salesAudit/store/fake/fake.go` swaps those variables for in-memory versions in tests. They filter with `core.MatchLead` / `core.MatchRecheck`. **When you change a filter, change both** `store/mongo.go` and `core/match.go`, or the tests will stop matching production.
- Nothing outside `store/` imports the Mongo driver.

## 9. Looking at the data yourself

Local dev database:

```bash
mongosh "mongodb://localhost:27017/audit_app"
```

```js
db.salesAuditMembers.find({}, { _id: 0, email: 1, role: 1, region: 1, managerEmail: 1 })

// Leads per audit status
db.salesAuditLeads.aggregate([{ $group: { _id: '$audit.status', n: { $sum: 1 } } }])

// Everything about one lead
const lead = db.salesAuditLeads.findOne({ zenId: '700020' })
db.salesAuditRechecks.find({ leadId: lead.id })
db.salesAuditAudits.find({ leadId: lead.id }).sort({ attempt: 1 })
db.salesAuditEvents.find({ leadId: lead.id }).sort({ at: 1 })

// Open tickets, newest first
db.salesAuditRechecks.find({ status: 'open' }).sort({ raisedAt: -1 })

// BDM → BDAs → leads
const bdm = 'bdm.north@example.com'
const bdas = db.salesAuditMembers.find({ role: 'bda', managerEmail: bdm, deleted: false }).map((m) => m.email)
db.salesAuditLeads.countDocuments({ bdaEmail: bdas[0], deleted: false })                                   // one BDA's leads
db.salesAuditLeads.countDocuments({ $or: [{ bdmEmail: bdm }, { bdaEmail: { $in: bdas } }], deleted: false }) // all the BDM's leads

// Leads per BDA under each BDM
db.salesAuditLeads.aggregate([
  { $match: { deleted: false } },
  { $group: { _id: { bdm: '$bdmEmail', bda: '$bdaEmail' }, leads: { $sum: 1 } } },
  { $sort: { '_id.bdm': 1, leads: -1 } },
])
```

Seed members and fake leads: see README section 1, *Dev data*. Create the indexes with `salesAudit/scripts/indexes.js`.
