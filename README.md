# Audit Checker Backend

Go backend for **Zen Sales Audit** (frontend: `AuditorClient`).

The flow:
1. Zoho sends every enrolment (a lead).
2. The portal assigns the lead to an auditor of its region.
3. The auditor audits it against the confirmation call (CC). They either complete the audit with a checklist, or raise a **recheck**.
4. The BDA fixes the recheck and closes the ticket, and the auditor audits the lead again.
5. The loop repeats until the audit is completed.

Every step goes on the lead's timeline, with who did it and when.

Roles: **Auditor TL**, **Auditor**, **BDM**, **BDA** (from `salesAuditMembers`).

The whole feature lives in `salesAudit/` and is written in a plain functional style: package-level functions, with no handler structs and no store interfaces. The store's functions are package variables, so tests swap them for an in-memory fake (`salesAudit/store/fake`) and need no database.

---

## Contents

1. [Getting started](#1-getting-started)
2. [Environment variables](#2-environment-variables)
3. [The flow](#3-the-flow)
4. [API](#4-api)
5. [Data](#5-data)
6. [Background jobs and mail](#6-background-jobs-and-mail)
7. [Project structure](#7-project-structure)
8. [Tests](#8-tests)
9. [Changes outside the feature folder](#9-changes-outside-the-feature-folder)
10. [Known gaps](#10-known-gaps)

---

## 1. Getting started

Requirements: **Go 1.25**, MongoDB, Redis.

```bash
go mod download
cp .env.example .env              # or export the variables in section 2
go run main.go                    # HTTP server on 127.0.0.1:$PORT
go run main.go -worker            # Redis worker: Zoho import + assignment, sweeps, mail delivery
go run main.go -with-worker       # both in one process (the Docker default)
```

### Docker / Render

The `Dockerfile` builds one image with Redis bundled in:
- `docker-entrypoint.sh` starts that Redis (in memory, on `127.0.0.1:6379`) whenever `REDIS_HOST` is unset or points at localhost, then runs the app.
- By default (`-with-worker`), one container runs the HTTP server **and** the worker.

```bash
docker build -t audit-app .
docker run --env-file .env -p 8080:8080 audit-app               # server + worker + bundled Redis (default)
```

On Render:
1. Create a **Web Service** from this repo, with runtime **Docker**. Leave the Docker Command empty.
2. Set the health check path to `/health`.
3. Add the section 2 variables under *Environment*. `.env` is not copied into the image.
4. Leave `REDIS_HOST` unset (or `localhost:…`) to use the bundled Redis.

Render sets `PORT` itself. MongoDB Atlas must allow Render's outbound IPs.

Notes on the bundled Redis:

- It lives in the container's memory, so jobs waiting in the queue are lost on a restart or deploy. The scheduled jobs are re-created when the worker starts, so they carry on.
- Free instances sleep when idle, which also pauses the worker and its jobs.
- It only works when the API and the worker are in the same container. To run them as separate services, use:
  - a Web Service with Docker Command `/app/audit-app`
  - a Background Worker with `/app/audit-app -worker`

  Then set `REDIS_HOST` (and `REDIS_PASSWORD`) on both to one external Redis, such as Render Key Value. The bundled Redis is then not started.

### Dev data

Indexes (safe to run more than once):

```bash
mongosh "$MONGO_URI/$MONGO_DATABASE" salesAudit/scripts/indexes.js
```

Fake members, for a **dev database only** (no real PII). Sign in as any of them from the frontend's dev shell with the token `dev-mock-token:<email>`:

```bash
mongosh "mongodb://localhost:27017/audit_app" salesAudit/scripts/seed.js
```

Fake leads: generate a Zoho-shaped response and import it as the auditor TL. Leads are assigned by region straight after: with 100 leads, the 20 North leads go to the North auditor and the South leads split 40/40.

```bash
node salesAudit/scripts/fakeZohoResponse.js 100 > /tmp/zoho.json
curl -X POST http://localhost:8080/sales-audit/zoho/import \
  -H "Authorization: dev-mock-token:tl@example.com" --data-binary @/tmp/zoho.json
```

Never import real Zoho exports into a dev database, and never commit them.

## 2. Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `HOST`, `PORT` | `127.0.0.1`, `8080` | HTTP listen address |
| `MONGO_URI`, `MONGO_DATABASE` | `mongodb://localhost:27017`, `audit_app` | MongoDB |
| `REDIS_HOST`, `REDIS_PASSWORD` | `localhost:6379`, empty | Redis for the worker queue |
| `SALES_AUDIT_PROGRAM` | `guvi` | The program (tenant) the mock auth puts in the context |
| `ACCOUNTS_EMAIL` | empty | Copied on payment escalation mails |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | `SMTP_PORT` is `587`; the others are empty | Mail delivery. With `SMTP_HOST` empty, mails are logged and marked `skipped`. |
| `ZOHO_API_URL` | the Zoho Creator learner API | Learner import |
| `ZOHO_API_PUBLIC_KEY` | empty | Zoho public key. The import fails without it. |
| `ZOHO_SYNC_LOOKBACK_DAYS` | `3` | The import fetches enrolments from the last N days |
| `ZOHO_API_FROM`, `ZOHO_API_TO` | empty | Set both (DD-Mon-YYYY) to fetch a fixed window instead |

## 3. The flow

```
Zoho import ──► lead (salesAuditLeads) ──► auto-assign by region (North / South, least loaded, ties random)
     │                                           │  notification + mail to each auditor
     │  CC link arrives (Superleap → Zoho)       ▼
     └─► cc.status pending → updated ──► auditor alerted: "CC can be verified"
                                                 ▼
             Audit: database vs CC side by side (+ CC verify: PDF or call transcript)
                 │                                          │
      Checklist (all ticked + comments)            Recheck (category + comments)
                 │                                          │  RC-000123 → alert + mail to BDA and BDM
                 ▼                                          ▼
          audit completed                 ticket open ──► BDA / BDM / auditor closes it (closer recorded)
                                                            │  auditor alerted: "audit again"
                                                            └──► recheckClosed ("audit pending") ──► audit again
```

A lead's audit status moves like this:
- `unassigned → pending → completed`, or
- `pending → recheckOpen → recheckClosed → completed`, or back to `recheckOpen` for another round.

A lead can only be completed when all of these hold:
- Every payment is verified, and the plan's minimum is met: the full fee for full payment, ₹15,000 for subscriptions, 40% of the fee for EMI.
- No recheck is open.
- Every checklist item is ticked.

Who sees what:

| Role | Leads | Rechecks | Dashboards |
|---|---|---|---|
| Auditor TL | All | All | Team dashboard (auditors reporting to them); members |
| Auditor | All Leads: all. My Leads: assigned. | Their own by default, all on request | None |
| BDM | Their BDAs' leads, and leads that name them as BDM | Same | BDA dashboard, per BDA |
| BDA | Their own | Their own | Their own |

Zoho's existing `auditCoordinator`, `auditStatus` and `recheckDetails` are imported once, as the starting state. After that the portal owns assignment, audit status and rechecks. Nothing is written back to Zoho.

## 4. API

All routes:
- are under `/sales-audit`
- take `Authorization: <token>` (no `Bearer`)
- answer `{"status":"success","data":…}` or `{"status":"error","message":…}`, with 400 / 401 / 403 / 404 / 500 on errors
- declare `salesAudit.view` or `salesAudit.edit`. Role checks happen in the actions.

Date filters take either:
- a preset `…In=today|thisWeek|lastWeek|thisMonth|lastMonth` (IST; weeks start on Monday), or
- `…From` / `…To` in Unix seconds.

| Method | Path | Permission | Who | What |
|---|---|---|---|---|
| GET | `/me` | view | all | The member, `permissions`, `teamEmails` |
| GET | `/members?role=` | view | all | Roster |
| POST | `/members` | edit | TL | Add a member `{name, email, userHash, role, region, managerEmail, available}` |
| PUT | `/members/:memberId` | edit | TL; an auditor may flip their own `available` | Edit a member |
| GET | `/leads` | view | scoped | Paged list. Filters: `scope=mine\|all`, `auditStatus` (comma list), `region`, `auditorEmail`, `bdaEmail`, `ccStatus`, `search`, `unassigned`, `completed…`, `recheckRaised…`, `recheckClosed…`, `recheckCategory`, `recheckStatus`, `awaitingReaudit`, `page`, `pageSize` |
| POST | `/leads/assign` | edit | TL | Assign unassigned leads now |
| GET | `/leads/:leadId` | view | scoped | The lead (personal, course, payment + discount, admission & T&C), its rechecks, its audits and the allowed `actions` |
| GET | `/leads/:leadId/timeline` | view | scoped | Events, oldest first, with actor and time |
| GET | `/leads/:leadId/audit` | view | auditors | Side-by-side `comparison`, `mismatchCount`, CC points covered, checklist, rechecks, `actions` |
| GET | `/leads/:leadId/cc-verification` | view | auditors | The lead plus the CC extract: a PDF `previewUrl` or a call `transcript` |
| GET | `/leads/:leadId/alerts` | view | scoped | The lead's mail log |
| POST | `/leads/:leadId/complete-audit` | edit | the lead's auditor, TL | `{checklist:[{key,checked}], comments}` |
| POST | `/leads/:leadId/reassign` | edit | auditors | `{auditorEmail}` |
| POST | `/leads/:leadId/take-up` | edit | auditor | Take the lead over |
| POST | `/leads/:leadId/send-reminder` | edit | auditors | Send the payment escalation mail now |
| GET | `/rechecks` | view | scoped | Filters: `scope`, `status=open\|closed`, `view=raisedNotClosed\|closedAuditPending\|closed`, `category`, `auditorEmail`, `bdaEmail`, `leadId`, `raised…`, `closed…` |
| POST | `/rechecks` | edit | the lead's auditor, TL | `{leadId, category, comments}`. Categories: `ccPending`, `payment`, `emi`, `approval`, `missedPointsInCc`, `downPayment` |
| POST | `/rechecks/:recheckId/close` | edit | the lead's BDA / BDM, auditors | `{note}` |
| GET | `/rechecks/cc-status?status=updated\|pending` | view | auditors | Leads by CC status (paged) |
| GET | `/dashboard/auditor-team?auditorEmail=&period…` | view | TL | Per auditor: assigned, open, audits done, completed, rechecks raised, per-day counts; recent audits |
| GET | `/dashboard/bda?bdaEmail=&period…` | view | BDA, BDM | Leads, completed audits, rechecks (open / closed / by category), per BDA |
| GET | `/notifications?unread=true` | view | all | `{items, unread}` |
| POST | `/notifications/:notificationId/read` | edit | all | Mark one notification read |
| POST | `/notifications/read-all` | edit | all | Mark all notifications read |
| GET | `/alerts?since=` | view | auditors | Mail log |
| POST | `/zoho/import` | edit | TL | Import a Zoho response (sent as the body), or fetch the sync window (empty body); then assign |
| POST | `/payment-verification/run-sweep` | edit | TL | Run the BDM payment-verification sweep now |
| POST | `/test-mail` | edit | TL | `{to}`: send one mail straight over SMTP |

## 5. Data

Every document has:
- `id` (a UUID string)
- `program`
- `created {at, by}` (Unix seconds, user hash)
- `deleted`

Every query filters on `program` and `deleted: false`. The indexes are in `salesAudit/scripts/indexes.js`.

| Collection | Holds | Key fields |
|---|---|---|
| `salesAuditLeads` | One document per Zoho enrolment | `zenId` (unique), `region`, `personal`, `course{batch}`, `payment{records, emis, discounts, partialReminders, subscriptions, ready, shortfall}`, `admission`, `termsAccepted`, `bdaEmail`, `bdmEmail`, `cc{link, type, status, updatedAt}`, `assignment{auditorEmail, mode}`, `audit{status, attempt, completedAt, completedBy}`, `recheckSummary` |
| `salesAuditMembers` | Who uses the feature | `userHash`, `email` (unique), `role`, `region`, `managerEmail`, `available` |
| `salesAuditAudits` | Each audit attempt | `leadId`, `attempt`, `auditor`, `outcome` (`completed` / `recheckRaised`), `checklist`, `comments`, `mismatchCount` |
| `salesAuditRechecks` | Recheck tickets | `recheckNo` (unique: `RC-000123`, or Zoho's SRID), `source`, `leadId`, `category`, `comments`, `raisedBy`, `raisedAt`, `bdaEmail`, `bdmEmail`, `auditorEmail`, `status`, `closed{at, by{email, name, role}, note}`, `reauditedAt` |
| `salesAuditEvents` | The lead timeline | `leadId`, `type`, `actor`, `at`, `data` |
| `salesAuditNotifications` | In-app alerts | `recipientEmail`, `type`, `leadId`, `recheckId`, `title`, `message`, `read` |
| `salesAuditCounters` | Sequences | `name`, `value` (recheck numbers) |
| `salesAuditAlerts` | Mail log | `leadId`, `kind`, `to`, `subject`, `body`, `trigger`, `sentAt`, `delivery` |
| `salesAuditCcExtracts` | What was read from the CC | `leadId` (unique), `link`, `type`, `fields`, `transcript`, `pointsCovered`, `mocked` |

Each import refreshes a lead's Zoho-owned fields. It never overwrites `assignment`, `audit` or `recheckSummary`.

## 6. Background jobs and mail

The jobs are registered by `salesAudit/worker.Register(pool)` (gocraft/work on Redis):

| Job | Schedule | Does |
|---|---|---|
| `salesAudit_zoho_import` | every 15 min | Fetches the sync window, imports it, then assigns unassigned leads |
| `salesAudit_assign_leads` | on demand | Assignment only |
| `salesAudit_escalation_sweep` | hourly | Mails the BDA, BDM and Accounts about leads with an unverified payment for over 24h; repeats daily |
| `salesAudit_recheck_reminder_sweep` | hourly | Mails the BDA and BDM again about rechecks open for over 24h; repeats daily |
| `salesAudit_payment_verification_sweep` | every 10 min | Mails the BDM once per lead about a payment unverified for over 24h |
| `salesAudit_send_mail` | queued | Delivers one logged mail over SMTP |

Mails the feature sends, besides the sweeps above:

| Mail | Goes to | Contents |
|---|---|---|
| Recheck raised | BDA + BDM | The recheck ID and the lead |
| Recheck closed | Auditor | |
| Leads assigned | Each auditor | One digest per auditor |
| CC updated | Auditor | |

Each mail is logged in `salesAuditAlerts`, with its body, before it is queued.

## 7. Project structure

```
salesAudit/
  models/        documents, enums, collection names, Zoho payload, API shapes
  core/          pure functions: Zoho mapping, payment rules, status machine, roles and scope,
                 assignment, date presets, CC comparison, checklist, dashboards, mail texts
  store/         the only package that talks to Mongo: exported function variables
  store/fake/    in-memory versions for tests (fake.Install(t))
  actions/       use cases: import, assign, audit, rechecks, dashboards, notifications, sweeps
  controllers/   Gin handlers, mock auth + member middleware
  routes/        Register(engine)
  worker/        SetupQueue + Register(pool) and the job functions
  zoho/          Zoho learner API client (resty)
  cc/            CC reader; Extract is a mock until the FastAPI service is connected
  scripts/       indexes.js, seed.js, fakeZohoResponse.js
```

## 8. Tests

```bash
go vet ./... && go test ./salesAudit/...
```

- `core/`:
  - assignment (20 North + 80 South leads split 20 / 40 / 40)
  - payment rules, status machine, date presets
  - scopes, Zoho mapping, CC comparison
- `controllers/`: every flow over the real routes, with the fake store:
  - auth; import and assignment
  - audit → recheck → close → re-audit → complete
  - timeline, filters, dashboards
  - reassign and take up, CC status
  - role denials

## 9. Changes outside the feature folder

Outside `salesAudit/` only the app shell is left:
- `main.go`: starts the HTTP server (`-with-worker` adds the worker; `-worker` runs only the worker).
- `config/config.go`: env vars and the Mongo connection.
- `controller/controller.go`: `GET /health`.
- `routes/routes.go`: `/health`, then `salesAuditWorker.SetupQueue` and `salesAuditRoutes.Register(router)`.
- `worker/start.go`: the worker pool running `salesAuditWorker.Register(pool)`.
- `go.mod`: adds `github.com/go-resty/resty/v2` (RULES §6). The JWT and bcrypt dependencies are gone.

**Removed** (replaced by `salesAudit/`, or unused by it):
- The legacy JWT auth (`POST /register`, `POST /login`, `GET /me`, `JWT_SECRET`) and the `/sap/*` CRUD with its `-sap-worker` mode: `controller/auth.go`, `controller/sap_controller.go`, `middleware/auth.go`, `models/`, `worker/sap_worker.go`, `worker/helloworld.go`
- `controller/sales_audit*.go`
- `middleware/sales_audit.go`
- `models/sales_audit*.go`
- `worker/sales_audit_*.go`, `worker/payment_verification*.go`, `worker/zoho_learner_import*.go`
- `scripts/sales_audit_*.js`

**Other notes:**
- The feature no longer reads or writes the `LeadData`, `paymentData`, `EmiData`, `DiscountData`, `PartialReminders` and `SubscriptionReminders` collections. Zoho data now lives in `salesAuditLeads`.
- On merge, Zen's auth middleware replaces `controllers.Auth()`. `controllers.Member()` then maps the user hash to a `salesAuditMembers` row, so members need their Zen `userHash`.

## 10. Known gaps

1. **CC reading is mocked.** `cc.Extract` replays the lead's own data, with a deterministic mismatch on about one lead in four, plus a sample transcript. Replace it with the FastAPI service client.
2. **Nothing is written back to Zoho**: not the assignment, the audit status or the rechecks.
3. **Mock auth.** Permissions are declared, not enforced. Roles come from `salesAuditMembers`.
4. **Dashboards aggregate in memory.** That is fine for thousands of leads; beyond that they need Mongo aggregation.
5. **Rechecks imported from Zoho as closed count as already re-audited.** There is no portal audit for them.
6. **Old dev data does not fit the new shape.** `salesAuditRechecks` and `salesAuditAudits` data from the previous version needs clearing once. `indexes.js` drops the old unique `{program, leadId}` index on `salesAuditAudits`.
