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

**Not yet built** (next milestone): the event-reminder send screen itself.
The current code only manages the member list and opt-in flow. Reminder
sending will hook into the [romelegion.org events JSON feed](https://github.com/howarthTech/legion-rome)
(which doesn't exist yet either) and let the admin send to all opted-in
members for a chosen event.

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

What's **not** implemented yet but should be before any real launch:

- A "Quiet hours" check (no SMS between, say, 9 PM and 9 AM local time).
- Per-member consent capture timestamps surfaced in the UI (we record them
  in the DB but don't yet show them prominently).
- A re-confirmation flow if a member doesn't reply within 24 hours.

---

## Production deployment (not yet enabled)

Per the [OPS hosting-pattern runbook](https://github.com/howarthTech/legion-rome/blob/main/runbooks/hosting-pattern.md),
this is a separate tenant from romelegion.org:

- `/srv/apps/legion-rome-crm/` on the VPS
- New compose file (Go container + named-volume SQLite)
- Resource budget (TBD with OPS — likely well under 100 MB RAM)
- Caddy site block routing `admin.romelegion.org` → `127.0.0.1:8081`
- Secrets at `/srv/secrets/legion-rome-crm.env` (mode 600, root-owned)
- Backup drop-in tars the SQLite file nightly

A Dockerfile, compose file, and Caddy block will land in a future commit
once we're ready to deploy. The current goal is local-only iteration.

---

## License

MIT — see LICENSE.
