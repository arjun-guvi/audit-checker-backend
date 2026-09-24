# Audit Checker Backend

Go backend for the **Zen Sales Audit** frontend (`AuditorClient`). It reads the sales data imported
from Zoho Creator, and stores what the audit team adds on top: verify decisions, rechecks, the BDA's
CC answers and a log of every alert mail. Scheduled jobs chase stuck leads and open rechecks by
mail.

The feature is written as plain functions in the repo's existing packages (`config`, `models`,
`middleware`, `controller`, `routes`, `worker`), in the same style as the `/sap` code: Mongo is
reached through `config.MongoDB`. Its files are prefixed `sales_audit`. The older JWT auth and
`/sap` CRUD code are unrelated to it.

---

## Contents

1. [Getting started](#1-getting-started)
2. [Environment variables](#2-environment-variables)
3. [API](#3-api)
4. [Data](#4-data)
5. [Background jobs and mail](#5-background-jobs-and-mail)
6. [Project structure](#6-project-structure)
7. [Tests](#7-tests)
8. [Changes to existing code](#8-changes-to-existing-code)
9. [Known gaps](#9-known-gaps)
10. [Legacy endpoints](#10-legacy-endpoints)

---

## 1. Getting started

Requirements: **Go 1.25**, MongoDB, Redis.

```bash
go mod download
cp .env.example .env              # or export the variables in section 2
go run main.go                    # HTTP server on 127.0.0.1:$PORT
go run main.go -worker            # Redis worker: Sales Audit sweeps + mail delivery
```

Indexes (safe to run more than once):

```bash
mongosh "$MONGO_URI/$MONGO_DATABASE" scripts/sales_audit_indexes.js
```

Fake data for a **dev database only** (replaces its previous seed on each run; no real PII):

```bash
mongosh "mongodb://localhost:27017/audit_app_dev" scripts/sales_audit_seed.js
```

Run the frontend against it: in `AuditorClient`, copy `.env.example` to `.env.local` and
`npm run dev`. The Vite dev server proxies `/api/*` to this server, so no CORS changes are needed.

## 2. Environment variables

| Variable | Default | Used for |
|---|---|---|
| `PORT` | `8080` | HTTP port |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection |
| `MONGO_DATABASE` | `audit_app` | Database holding the Zoho and `salesAudit*` collections |
| `REDIS_HOST` | `localhost:6379` | Job queue |
| `REDIS_PASSWORD` | – | Job queue |
| `SALES_AUDIT_PROGRAM` | `guvi` | `program` set by the mock auth middleware and used by the jobs |
| `ACCOUNTS_EMAIL` | – | Accounts address copied on "payment pending over 24h" mails |
| `SMTP_HOST`, `SMTP_PORT` (`587`), `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | – | Mail delivery. When unset, mails are still logged (and shown in the UI) but marked `skipped` |
| `JWT_SECRET` | – | Legacy `/me` auth only |

Never commit `.env`.

## 3. API

All routes are under `/sales-audit`. The client sends `Authorization: <token>` (no `Bearer`).
Responses are `{"status":"success","data":…}` or `{"status":"error","message":…}` with 400 / 401 /
404 / 500. Full request/response shapes: `AuditorClient/API_ENDPOINTS.md`.

| Method | Path | Permission | Does |
|---|---|---|---|
| GET | `/leads` | `salesAudit.view` | Leads in the `Audit` stage with credits, escalation, CC answer and audit; `mailsSentThisSweep` = automatic escalation mails in the last hour |
| GET | `/leads/summaries` | `salesAudit.view` | Lead picker / CC Status list |
| POST | `/leads/:leadId/send-reminder` | `salesAudit.edit` | Mail the BDA and Accounts now; 400 if nothing to escalate |
| POST | `/leads/:leadId/cc-response` | `salesAudit.edit` | `{response: "mailSentAwaitingAck" \| "mailNotSent"}`; `mailNotSent` alerts the BDA; 400 if the CC is already uploaded |
| GET | `/leads/:leadId/audit` | `salesAudit.view` | Lead Audit Workspace: Zoho / CC / EMI vendor sources, rechecks, latest discount request |
| POST | `/leads/:leadId/mark-audited` | `salesAudit.edit` | `{overrideReason}`; moves the lead to Awaiting; 400 if already verified |
| GET | `/rechecks` | `salesAudit.view` | All rechecks, newest first |
| POST | `/rechecks` | `salesAudit.edit` | `{leadId, category, notes}`; alerts the BDA and BDM |
| POST | `/rechecks/:recheckId/resolve` | `salesAudit.edit` | Mark resolved; 400 if already resolved |
| GET | `/audit-history?from=<unix s>` | `salesAudit.view` | Rechecks, payments and alert mails for trends (default: last 60 days) |
| GET | `/students/:studentId` | `salesAudit.view` | One lead (any stage) |
| GET | `/students/:studentId/payments` | `salesAudit.view` | The lead's financial records, oldest first |
| GET | `/students/:studentId/cc-verification` | `salesAudit.view` | Zoho vs CC-mail comparison; 404 until the CC has been extracted |

`leadId` / `studentId` is the Zoho `ID` of the `LeadData` record.

## 4. Data

### Zoho collections (read-only)

Imported from Zoho Creator; this service only reads them and adds indexes.

| Collection | Joined by | Used for |
|---|---|---|
| `LeadData` | `ID` (lead id), `zen_id` | Leads (`Stage = "Audit"`), SAP clock (`Added_Time`), CC link and upload time (`Confirmation_Call_*`), BDA (`Sale_Owner`), BDM (`Sale_Owner_s_Manager`), course / batch / fee fields |
| `paymentData` | `All_Enrolment` (or `Zen_ID`) = `zen_id` | Credits: latest `Credit_Booking_Amount` / `Credit_Part1` / `Credit_RemainingBalance` and their `Verified`; payment timings (`Added_Time`, `Verified_on`) |
| `EmiData` | `Zen_ID` | EMI vendor side of the audit (loan amount, first EMI, ROI, tenure, status, vendor) |
| `PartialReminders` | `Student_ID` = lead `ID` | Zoho installment plan for partial payments |
| `SubscriptionReminders` | `Zen_ID` | Zoho installment plan for subscriptions |
| `DiscountData` | `Learner_Email_ID` = lead `Email` | "Discount approved" check |

Zoho dates come in several layouts (`15-Sep-2026 15:22:19`, `19-Sep-2026`, `2026-09-24`, ISO); they
are read as IST unless they carry a zone (`worker/sales_audit_format.go`).

### Feature collections

Every document has `id` (UUID), `program`, `created: {at, by}` and `deleted`; timestamps are Unix
seconds; every query filters on `program` and `deleted: false`.

| Collection | Fields | Indexes |
|---|---|---|
| `salesAuditRechecks` | `leadId, category, notes, status (open\|resolved), raisedBy, raisedAt, resolvedAt, alert, lastReminder` | `{program, id}` unique; `{program, deleted, leadId, raisedAt}`; `{program, deleted, status}` |
| `salesAuditAlerts` | `leadId, kind (escalation\|recheck\|recheckReminder\|ccNotSent), to, subject, trigger (auto\|manual), sentAt, delivery (queued\|sent\|skipped\|failed)` | `{program, id}` unique; `{program, deleted, leadId, kind, sentAt}`; `{program, deleted, sentAt}` |
| `salesAuditCcResponses` | `leadId, response, updatedAt, alert` | `{program, leadId}` unique |
| `salesAuditAudits` | `leadId, auditedAt, auditedBy, overrideReason` | `{program, leadId}` unique |
| `salesAuditCcExtracts` | `leadId, scraped, pointsCovered` (written by the future CC parser) | `{program, leadId}` unique |

## 5. Background jobs and mail

Jobs use the existing gocraft/work Redis pool (`worker/start.go`, namespace `audit_worker`).

| Job | When | Does |
|---|---|---|
| `salesAudit_sap_escalation_sweep` | Hourly | Leads over 24h in SAP with an unverified or mismatched payment, not mailed in the last 24h → mail BDA + Accounts |
| `salesAudit_recheck_reminder_sweep` | Hourly | Rechecks open 24h since raised or last reminded → mail BDA + BDM, set `lastReminder` |
| `salesAudit_send_mail` | Enqueued by the API and sweeps | Sends one logged alert over SMTP and records its `delivery` |

A mail is logged in `salesAuditAlerts` first and then queued, so the UI shows it even if Redis or
SMTP is down (its `delivery` then reads `failed` or `skipped`).

## 6. Project structure

```
config/config.go                  SALES_AUDIT_PROGRAM, ACCOUNTS_EMAIL, SMTP_* settings
models/sales_audit.go             collection names, permissions, enums, feature documents
models/sales_audit_zoho.go        read structs for the Zoho collections (Zoho field names)
models/sales_audit_api.go         response shapes the frontend reads
models/worker.go                  job names (SALES_AUDIT_*_JOB)
middleware/sales_audit.go         SalesAuditAuth (mock auth), RequirePermission
controller/sales_audit.go         one handler function per endpoint
routes/routes.go                  /sales-audit group, permission per route
worker/sales_audit_db.go          every Mongo read/write (the Zoho collections are only read)
worker/sales_audit_leads.go       shared operations: build leads, schedule, discount, log + queue mail
worker/sales_audit_jobs.go        hourly sweeps, mail job, SMTP delivery, job registration
worker/sales_audit_format.go      pure: Zoho date parsing, formatting, UUIDs
worker/sales_audit_rules.go       pure: escalation / reminder rules, recipients, subjects
worker/sales_audit_mapping.go     pure: Zoho -> API mapping, audit sources
scripts/sales_audit_indexes.js    indexes (idempotent)
scripts/sales_audit_seed.js       fake data for a dev database
```

The shared operations live in `worker` because both the handlers and the jobs use them, and
`controller` already imports `worker` (as the `/sap` code does).

## 7. Tests

```bash
go test ./...                                              # pure rules and mapping
TEST_MONGO_URI=mongodb://localhost:27017 go test ./...     # + every endpoint and job
go vet ./...
```

The endpoint tests (`controller/sales_audit_test.go`) and job tests
(`worker/sales_audit_jobs_test.go`) run the real routes and queries against a throwaway database
that each test creates and drops; they skip when `TEST_MONGO_URI` is not set. Mail queueing, SMTP
and the clock are replaced in tests.

## 8. Changes to existing code

- `main.go`: `workerNamespace` constant; passes the Redis pool and namespace to `SetupRoutes`.
- `config/config.go`: Sales Audit and SMTP settings.
- `models/worker.go`: Sales Audit job names.
- `routes/routes.go`: `SetupRoutes(router, redisPool, workerNamespace)` mounts `/sales-audit`.
- `worker/start.go`: registers the Sales Audit jobs on the worker pool.
- `go.mod`: `go 1.25` (was 1.26.4, which RULES.MD does not allow); `golang.org/x/*` pinned to
  versions that build on Go 1.25.
- Removed: the committed macOS binaries `audit-app` and `tmp/main`, and the empty
  `models/models.go` and `worker/worker.go`.

## 9. Known gaps

1. **No `program` on the Zoho collections**, so reads from them cannot be filtered by tenant. The
   feature collections are. Needs a `program` field on the import, or a separate database per
   program.
2. **CC mail is not parsed.** `sources.cc` stays `null` and `cc-verification` returns 404 until
   something (the AI service via this backend) writes `salesAuditCcExtracts`.
3. **No real EMI vendor feed.** The vendor side is the `EmiData` loan application. Zoho's own
   EMI side has only the tenure (from `Payment_Type`) and status to compare against it.
4. **Mock auth.** `auth` is the raw token and permissions are declared but not enforced; Zen's
   middleware replaces both. `raisedBy` / `auditedBy` read "Audit Team" until Zen gives user names.
5. **Installment alignment is inferred.** For split plans the reminders are assumed to cover the
   parts still due, with the initial payment (`Credit_Part1`) as the first part.
6. `mailsSentThisSweep` counts automatic escalation mails from the last hour, so the frontend's
   toast repeats on every load during that hour.
7. No undo for "mark audited", no server-side re-check of the audit checklist.

## 10. Legacy endpoints

Pre-existing and unrelated to the Sales Audit feature: `POST /register`, `POST /login`,
`GET /me` (JWT, `Authorization: Bearer <token>`), `/sap/*` CRUD and the `-sap-worker` mode.
