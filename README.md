# legion-rome-crm

Admin CRM for **Shanklin Attaway American Legion Post 5** (Rome, GA).
Manages the SMS reminder list and sends event reminders to opted-in members
via Twilio.

A sibling of [romelegion.org](https://github.com/howarthTech/legion-rome) (the
public website). They share no code — the website is a static Hugo site,
this is a small Go web app.

```
┌──────────────────────────────────────────────────────────────────┐
│  Admin (one set user)                                            │
│    │                                                             │
│    ▼ login                                                       │
│  ┌────────────────────┐         ┌──────────────────┐             │
│  │ legion-rome-crm    │────────▶│  Twilio REST API │ outbound    │
│  │  (Go + SQLite)     │         └──────────────────┘             │
│  │                    │◀────────┐                                │
│  └────────────────────┘         │  webhook                       │
│         ▲                       │                                │
│         │                  ┌──────────────────┐                  │
│         │  signed cookie   │ Twilio carrier   │  STOP / YES      │
│         │                  │ (real SMS path)  │◀───── Member's   │
│         │                  └──────────────────┘       phone      │
│  Caddy on host @ admin.romelegion.org                            │
└──────────────────────────────────────────────────────────────────┘
```

---

## What it does

- One admin signs in.
- Admin adds members (name + phone).
- The CRM sends each new member a **TCPA-compliant opt-in SMS** asking them
  to reply `YES` to confirm. Until they confirm, they're `PENDING` and won't
  receive anything else.
- Members reply `YES` → status becomes `OPTED_IN`.
- Members reply `STOP` (or any of the standard opt-out keywords) → status
  becomes `OPTED_OUT`.
- Only `OPTED_IN` members are eligible for event reminders.
- Every SMS (in and out) is audit-logged per member.

- The **event-reminder send screen** (`/reminders`) reads the site's
  [events JSON feed](https://github.com/howarthTech/legion-rome) and sends a
  reminder to every `OPTED_IN` member for a chosen event — guarded by quiet
  hours (9 AM–9 PM in `ORG_TIMEZONE`) and audit-logged like everything else.

---

## Stack

- **Go 1.23+** — single static binary, ~18 MB.
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
  (pure Go, no CGO).
- **net/http** (stdlib) + Go 1.22 routing patterns.
- **html/template** for server-rendered admin UI.
- **Twilio Programmable Messaging** REST API (no SDK — the surface we use is
  ~3 endpoints).

Everything is embedded into the binary at compile time — migrations,
templates, CSS. The runtime artifact is one binary + one SQLite file.

---

## Local development

You'll need Go 1.23+. No other tooling required.

```bash
# 1. Configure
cp .env.example .env
make hash-password PW='choose-a-password'       # paste output → ADMIN_PASSWORD_HASH in .env
make gen-secret                                 # paste output → SESSION_SECRET in .env
# Leave the TWILIO_* vars empty for now — the SMS client runs in dry-run mode
# (logs to stdout instead of calling Twilio) when credentials are missing.

# 2. Run
make build
make run
# → server on http://localhost:8081
```

Sign in at http://localhost:8081/login with `admin` + your chosen password.

### Testing the opt-in flow without Twilio

1. Add a member at `/members/new`. The "opt-in SMS" prints to the server's
   stdout instead of going to a phone.
2. Simulate the member's reply by calling the webhook yourself:
   ```bash
   curl -X POST \
     --data-urlencode "From=+17065551234" \
     --data-urlencode "Body=YES" \
     --data-urlencode "MessageSid=SM-test" \
     http://localhost:8081/webhooks/twilio
   ```
3. Refresh the member detail page — status should now be **Opted in**.

To test against real Twilio, fill in `TWILIO_*` in `.env` and point Twilio's
"Messaging webhook" at your tunneled URL (ngrok, Cloudflare Tunnel, etc.).

---

## Repo layout

```
.
├── cmd/
│   ├── server/                 main entry point + embedded web/ assets
│   │   ├── main.go
│   │   └── web/
│   │       ├── templates/      html/template files
│   │       └── static/         CSS + (future) JS
│   └── hash-password/          tiny utility to generate bcrypt hashes
├── internal/
│   ├── app/                    App struct (deps wiring) + template helpers
│   ├── auth/                   Session-cookie auth (HMAC-signed)
│   ├── handlers/               HTTP handlers (one file per group)
│   │   ├── auth.go             login / logout
│   │   ├── members.go          CRUD + opt-in dispatch
│   │   └── webhook.go          Twilio inbound (YES/STOP)
│   ├── sms/                    Twilio REST client + webhook signature verify
│   └── store/                  SQLite-backed store
│       ├── store.go            connection + embedded migrations
│       ├── members.go
│       ├── messages.go
│       └── migrations/         SQL files, embedded into the binary
├── .env.example
├── go.mod
├── Makefile
└── README.md
```

---

## TCPA compliance — what's in place

The Telephone Consumer Protection Act (TCPA) gates almost every part of
this app. The choices that satisfy it:

1. **Express consent before any non-opt-in message.** Members start as
   `PENDING`. They only become eligible for reminders after they reply
   `YES`. The reminder-send code (TBD) will use `ListOptedIn`, which only
   returns `OPTED_IN` rows. Sending to a `PENDING` member is a bug.
2. **Opt-out instructions in every message.** The opt-in SMS includes
   *"Reply STOP to opt out."* — visible in [`internal/handlers/members.go`](./internal/handlers/members.go).
   Reminder messages (when built) will too.
3. **Honor STOP / UNSUBSCRIBE / CANCEL / END / QUIT.** The webhook handler
   in [`internal/handlers/webhook.go`](./internal/handlers/webhook.go)
   recognizes all common opt-out keywords and flips status immediately.
4. **No marketing.** Messages are strictly informational (event reminders).
5. **Audit trail.** Every inbound and outbound SMS is logged in
   `messages_log` so consent capture can be demonstrated if challenged.

6. **Quiet hours.** Reminders only send between 9 AM and 9 PM in the post's
   timezone (`ORG_TIMEZONE`) — see [`internal/events/quiethours.go`](./internal/events/quiethours.go).

What's **not** implemented yet but should be before any real launch:

- Per-member consent capture timestamps surfaced in the UI (we record them
  in the DB but don't yet show them prominently).
- A re-confirmation flow if a member doesn't reply within 24 hours.

---

## Multi-tenant: one image, many posts

This is the **shared CRM image** for the Legion Post Platform (see
[plan.md](https://github.com/howarthTech/legion-rome/blob/main/plan.md)). The
same binary/image runs every client; a tenant is entirely defined by its
environment + its SQLite volume. There is no per-client code and no
`tenant_id` — each post runs an isolated container with its own DB.

### Per-client env contract

Everything that differs between posts comes from these variables (full
reference in [`.env.example`](./.env.example)):

| Variable | Required | Per-client value |
|---|---|---|
| `ORG_NAME` | **yes** | The post's name — appears in every SMS body + page chrome. No default; startup aborts if unset (so one post's messages can't be branded with another's name). |
| `PUBLIC_URL` | yes | `https://admin.<post-domain>` — also used to verify Twilio webhook signatures. |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH` | yes | That post's admin login. |
| `SESSION_SECRET` | yes | Random ≥32 chars, unique per client. |
| `TWILIO_ACCOUNT_SID` / `TWILIO_AUTH_TOKEN` / `TWILIO_FROM_NUMBER` | for real sends | That post's Twilio subaccount + number. Empty → dry-run mode. |
| `DB_PATH` | no (default `./data/crm.db`) | Points at that client's mounted volume. |
| `EVENTS_FEED_URL` | for reminders | The site's `/events/events.json`. Empty → reminder screen unavailable. |
| `ORG_TIMEZONE` | no (default `America/New_York`) | Post timezone for the SMS quiet-hours guard. |
| `LISTEN_ADDR` | no | Local runs default `127.0.0.1:8081`; the **container image** defaults `0.0.0.0:8081` (don't override in a client env file). |

Provisioning a new post = generate this env file + a named volume + a Caddy
route; no rebuild.

## Container image

The shared image is published to **`ghcr.io/howarthtech/legion-rome-crm`** by
[`.github/workflows/publish.yml`](./.github/workflows/publish.yml) on every
push to `main` (tagged `latest` + `sha-<short>`; git tags publish `vX.Y.Z`).

```bash
# Build locally
docker build -t legion-rome-crm .

# Run a client instance: host port 8082 -> internal 8081, named volume for DB
docker run -d --name crm-post-x \
  --env-file /srv/secrets/crm-post-x.env \
  -p 127.0.0.1:8082:8081 \
  -v crm-post-x-data:/data \
  ghcr.io/howarthtech/legion-rome-crm:latest
```

The image always listens on `0.0.0.0:8081` inside the container; the host
restricts exposure by publishing to `127.0.0.1:<client-port>:8081`, and Caddy
reverse-proxies `admin.<domain>` to that host port. Runs as a non-root user
(uid 10001); the SQLite DB lives on the mounted `/data` volume. ~38 MB.

The provisioner ([legion-post-platform](https://github.com/howarthTech/legion-post-platform))
generates the per-client `docker-compose.snippet.yml` + env file that wire all
of this up.

## Production deployment

Per the [OPS hosting-pattern runbook](https://github.com/howarthTech/legion-rome/blob/main/runbooks/hosting-pattern.md),
each post is a separate tenant:

- Its own container from the shared image under `/srv/apps/crm-<client>/`
- Named-volume SQLite (`crm-<client>-data:/data`)
- Resource budget: `0.25` CPU / `128m` (set in the generated compose)
- Caddy block routing `admin.<domain>` → its published loopback port
- Secrets at `/srv/secrets/crm-<client>.env` (mode 600, root-owned)
- Backup drop-in tars the named volume nightly

**Remaining before first real deploy:** set the GHCR package to Public (or give
the VPS a read PAT), and have the OPS conversation about the multi-tenant
budget.

---

## License

MIT — see LICENSE.
