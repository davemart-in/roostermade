# RoosterMade — Platform Context Document

This document provides full context for AI agents and developers working on the RoosterMade platform. Read this before writing any code.

---

## What is RoosterMade?

RoosterMade is a platform of open source micro-SaaS tools built and maintained with the help of Elroy — a rooster who codes through the night while the farmer sleeps. Elroy is the creative personification of AI assistance underlying the platform. The software is serious. The narrative is fun.

Each RoosterMade product is:
- Open source at its base (self-hostable by anyone)
- Available as a hosted SaaS option at roostermade.com
- Built on a shared core library (`rooster-core`)
- Independently installable and deployable
- Designed to interoperate with other RoosterMade products
- AI-agent-ready via MCP (Model Context Protocol)

---

## Tech Stack — Non-Negotiable Constraints

- **PHP 8.1+** — functions only, no classes
- **SQLite** — one database file per app, WAL mode enabled
- **Vanilla JS** — no npm, no Node, no React, no build step, no frameworks
- **Vanilla CSS** — no Sass, no PostCSS, no Tailwind
- **HTML templates** — simple `{{variable}}` substitution, no template engines
- **No Composer dependencies** — no external PHP libraries
- **Local dev** — `php -S localhost:8000 -t public/`
- **HTTP calls** — `file_get_contents` + `stream_context_create` only

If a solution requires npm, Composer, or a build step, find a different solution.

---

## Platform Architecture

### Three Layers Per Product

Every RoosterMade product has three distinct layers, each separately deployed:

```
marketing/{app}/        # static HTML/CSS/JS — Cloudflare Pages or Netlify
apps/{app}/             # open source self-hosted tool
saas/{app}/             # hosted SaaS wrapper (thin layer on top of apps/{app})
```

These three layers share `core/` but are otherwise decoupled. The open source app has no marketing code. The marketing site has no app logic.

### Full Directory Structure

```
roostermade/
├── core/                           # shared platform core — NEVER EDIT
│   ├── php/
│   │   ├── router.php              # route registration and dispatch
│   │   ├── auth.php                # API key auth, magic link, sessions
│   │   ├── response.php            # send_json, send_success, send_error
│   │   ├── db.php                  # SQLite helpers
│   │   ├── email.php               # multi-provider email sending
│   │   ├── webhooks.php            # outbound webhook dispatch and retry
│   │   ├── stripe.php              # raw Stripe API helpers
│   │   └── migrate.php             # schema migration runner
│   ├── js/
│   │   ├── components.js           # component registry
│   │   ├── api.js                  # fetch wrapper
│   │   └── components/
│   │       ├── modal.js
│   │       ├── navbar.js
│   │       ├── form-input.js
│   │       ├── data-table.js
│   │       ├── toast.js
│   │       └── pagination.js
│   ├── css/
│   │   ├── variables.css
│   │   ├── components.css
│   │   ├── admin.css
│   │   └── utilities.css
│   ├── templates/
│   │   ├── components/
│   │   │   ├── modal.html
│   │   │   ├── navbar.html
│   │   │   ├── form-input.html
│   │   │   ├── data-table.html
│   │   │   ├── toast.html
│   │   │   ├── pagination.html
│   │   │   ├── card.html
│   │   │   └── stat-block.html
│   │   └── layouts/
│   │       ├── base.html
│   │       ├── admin.html
│   │       └── bare.html
│   ├── mcp/
│   │   ├── server.php              # stdio MCP transport handler
│   │   └── tools.php              # tool definitions and handlers
│   ├── VERSION                     # semver string e.g. 1.0.0
│   └── CHANGELOG.md
│
├── apps/
│   └── ledger/                     # first RoosterMade product
│       ├── core/                   # ledger app core — NEVER EDIT
│       │   ├── php/
│       │   │   ├── routes.php
│       │   │   ├── customers.php
│       │   │   ├── services.php
│       │   │   ├── transactions.php
│       │   │   ├── usage.php
│       │   │   ├── payments.php
│       │   │   ├── checkout.php
│       │   │   ├── webhooks.php
│       │   │   └── admin.php
│       │   ├── js/
│       │   │   ├── app.js
│       │   │   └── components/
│       │   │       ├── balance-display.js
│       │   │       └── transaction-row.js
│       │   └── css/
│       │       └── app.css
│       ├── templates/              # app-level template overrides
│       │   ├── components/
│       │   │   └── navbar.html
│       │   └── layouts/
│       │       └── base.html
│       ├── db/
│       │   ├── schema.sql
│       │   └── migrations/
│       │       └── 001_initial.sql
│       ├── data/                   # gitignored — ledger.db lives here
│       │   └── .gitkeep
│       └── install/
│       │   ├── install.sh
│       │   └── migrate.php
│    	├── tests/
│    	│   ├── runner.php
│    	│   ├── helpers.php
│    	│   ├── run.sh
│    	│   ├── test-db.php
│    	│   ├── test-auth.php
│    	│   ├── test-customers.php
│    	│   ├── test-transactions.php
│    	│   ├── test-usage.php
│    	│   ├── test-stripe.php
│    	│   ├── test-webhooks.php
│    	│   └── test-mcp.php
│
├── overrides/                      # user customizations — edit freely
│   └── ledger/                     # place override files here
│       ├── .gitkeep
│       ├── config/
│       │   ├── config.php
│       │   └── .env.example
│
├── marketing/
│   └── ledger/                     # static marketing site
│       ├── index.html
│       ├── pricing.html
│       ├── docs/
│       ├── demo/
│       │   └── index.html          # LocalStorage simulation of Ledger
│       ├── css/
│       └── js/
│
├── saas/
│   └── ledger/                     # hosted SaaS wrapper
│       ├── app/
│       │   └── php/
│       │       ├── signup.php
│       │       ├── billing.php
│       │       ├── tenants.php
│       │       └── routes.php
│       └── config/
└── public/
    └── index.php                   # single entry point — routes all requests
```

---

## The Override System

This is how customization works without touching protected files.

Every function in `core/php/` is wrapped in `if (!function_exists())`. Override files are loaded before core files. If you define a function in an override file, the core version is automatically skipped.

**Boot order in `public/index.php`:**

```php
function load_with_override(string $app, string $file): void {
    $override = "../overrides/{$app}/php/{$file}";
    $core     = "../apps/{$app}/core/php/{$file}";
    if (file_exists($override)) require_once $override;
    if (file_exists($core))     require_once $core;
}
```

**Template loading:** `template_render()` checks `apps/{app}/templates/` first, falls back to `core/templates/`. Simple `{{variable}}` substitution.

**Rules:**
- `core/` — never edit - Re-used across apps
- `apps/{app}/` — never edit
- `overrides/{app}/` — safe to edit (user customizations & config)

---

## Versioning

Each app is independently versioned. Core is versioned separately. Apps pin to a core version.

`rooster.json` at the app root is the machine-readable manifest:

```json
{
  "app": "ledger",
  "name": "RoosterMade Ledger",
  "version": "1.0.0",
  "core_version": "1.0.0",
  "requires": [],
  "stripe": true,
  "webhooks": true,
  "mcp": true,
  "env": {
    "required": ["APP_URL", "API_KEY", "STRIPE_SECRET_KEY",
                 "STRIPE_WEBHOOK_SECRET", "STRIPE_PUBLISHABLE_KEY",
                 "MAIL_PROVIDER", "MAIL_API_KEY", "MAIL_FROM"],
    "optional": ["MAIL_FROM_NAME", "APP_NAME"]
  }
}
```

Database schema is versioned via `schema_versions` table. Migrations are numbered SQL files in `db/migrations/`. `run_migrations()` applies unapplied migrations on boot.

Semantic versioning: major versions break public API, minor versions add safely, patches fix things. `core/CHANGELOG.md` must document every change to overridable functions.

---

## RoosterMade Ledger

The first product. The billing kernel that all other RoosterMade products can use.

### What it does

Ledger is a prepaid credit management system. Customers purchase credits via Stripe. Those credits are drawn down as they use services. One unified balance, one invoice, multiple services.

**Billing model: prepaid credits only.** If you need subscription billing, monthly allocations, or pay-in-arrears, Ledger is not the right tool.

### Credit Model

- Balances are **integers only** — whole credits, no decimals
- One **unified credit pool per customer** — not per service
- Credits are **non-refundable** once spent
- Each service has a **credit_cost** (e.g. Quill costs 2 credits per email, an image generation service might cost 50)
- Transaction log records **which service** made each debit for reporting purposes
- Billing is centralized — one invoice shows line items from all services

### Database Tables

| Table | Purpose |
|---|---|
| `schema_versions` | Migration tracking |
| `admin_users` | Admin panel users |
| `magic_codes` | Magic link auth codes |
| `services` | Registered micro-services with credit_cost |
| `customers` | End-users with unified credit balance |
| `transactions` | Every debit and credit with balance_after snapshot |
| `payments` | Stripe payment records — source of truth |
| `webhook_endpoints` | Registered outbound webhook URLs |
| `webhook_deliveries` | Delivery log with retry state |

### API Endpoints

All endpoints require `Authorization: Bearer {API_KEY}` except `POST /api/stripe/webhook`.

| Method | Path | Description |
|---|---|---|
| POST | `/api/customers` | Create customer |
| GET | `/api/customers/{id}` | Get customer + balance |
| GET | `/api/customers/{id}/invoice` | Usage + payments for date range |
| POST | `/api/customers/{id}/credit` | Manual credit |
| GET | `/api/customers/{id}/transactions` | Paginated transaction history |
| GET | `/api/customers/{id}/payments` | Payment history |
| POST | `/api/customers/{id}/checkout` | Create Stripe checkout session |
| GET | `/api/services` | List registered services |
| POST | `/api/services` | Register a service |
| POST | `/api/usage` | Record usage / debit credits (primary ingest) |
| POST | `/api/stripe/webhook` | Inbound Stripe events |
| POST | `/api/webhooks` | Register webhook endpoint |
| DELETE | `/api/webhooks/{id}` | Remove webhook endpoint |
| GET | `/api/webhooks/{id}/deliveries` | Delivery log |

### Critical: Atomic Debit

`POST /api/usage` must use a SQLite transaction (`BEGIN`/`COMMIT`) to atomically check balance and debit. The `transactions` table has a `UNIQUE` constraint on `(service_slug, reference_id)` for idempotency. Sending the same `reference_id` twice returns the original transaction without double-debiting.

### Stripe Integration

Scope for v1: prepaid credit purchase only.

Flow:
1. App calls `POST /api/customers/{id}/checkout` with `amount_cents` and `credits`
2. Ledger creates Stripe Checkout Session, returns `checkout_url`
3. Customer completes payment on Stripe
4. Stripe sends `checkout.session.completed` to `POST /api/stripe/webhook`
5. Ledger verifies signature, credits customer balance, records transaction
6. Ledger dispatches `payment.received` webhook

No subscriptions. No invoicing. No customer portal. Just this loop.

### Outbound Webhook Events

| Event | Trigger |
|---|---|
| `usage.recorded` | Successful debit |
| `balance.low` | Balance drops below `low_balance_threshold` |
| `balance.depleted` | Balance reaches zero |
| `balance.credited` | Manual credit applied |
| `payment.received` | Stripe payment confirmed |
| `debit.failed` | Insufficient funds |

Payloads signed with HMAC-SHA256. Delivery retried up to 5 times with exponential backoff.

### Admin UI

Server-rendered PHP. Vanilla JS only. Protected by magic link auth.

Pages: Dashboard, Customers, Customer Detail, Services, Transactions, Payments, Webhooks.

### Authentication

Admin panel uses magic link with 6-digit code. No passwords stored.

Flow: enter email → receive 6-digit code via email → enter code → session created.

- Codes stored as `hash_hmac('sha256', $code, APP_KEY)` — never plaintext
- Codes expire after 15 minutes, single use
- Max 3 code requests per email per 15 minutes
- "Remember this device for 30 days" checkbox sets long-lived signed cookie
- Session ID regenerated on login

### Email Sending

Configured via `MAIL_PROVIDER` env var. Supported providers: `resend`, `mailgun`, `postmark`, `ses`. All use raw HTTP POST — no SDK, no Composer. Falls back to `mail()` if not configured (unreliable — use a real provider).

For local development, use [Mailpit](https://github.com/axllent/mailpit) to catch outgoing email.

---

## MCP (Model Context Protocol)

Every RoosterMade app ships with an MCP server. This is not optional — MCP is core to the product's value proposition for AI agent workflows.

**v1 transport:** stdio (local Claude Code use only)  
**Protocol version:** 2024-11-05

### MCP Tools (Ledger)

| Tool | Description |
|---|---|
| `create_customer` | Create a new customer with zero balance |
| `get_customer` | Get customer details and current balance |
| `check_balance` | Check balance with low-balance flag |
| `debit_credits` | Debit credits before performing metered action |
| `credit_account` | Manually credit a customer's balance |
| `record_usage` | Alias for debit_credits |
| `get_transactions` | Paginated transaction history |
| `get_invoice` | Usage and payments for a date range |
| `list_services` | List active registered services |
| `register_service` | Register a new micro-service |
| `create_checkout_session` | Create Stripe checkout session |

**Critical for agents:** Tool descriptions are written for machines, not humans. They specify preconditions, postconditions, and exact behavior on every error response. `debit_credits` explicitly states: call before performing the metered action, do not proceed if response is `insufficient_funds`, return 402 to your caller.

### How Agents Discover and Install Ledger

1. Agent reads `rooster.json` — learns app name, dependencies, required env vars, MCP server location
2. Agent runs `apps/ledger/install/install.sh` — provisions database, creates first admin user
3. Agent reads `config/.env.example` — knows exactly what to populate
4. Agent runs `php core/mcp/server.php` — MCP server ready on stdio
5. Agent calls `tools/list` — receives all tool definitions
6. Agent integrates without reading additional documentation

---

## Adding a New RoosterMade App

When building a new product (e.g. Quill — email template management):

1. Create `apps/quill/` following the identical directory structure as `apps/ledger/`
2. Create `apps/quill/rooster.json` — set `requires: ["ledger"]` if it needs billing
3. `core/` is pulled in via git subtree — same version pin as other apps
4. App-specific logic lives in `apps/quill/core/php/`
5. App-specific templates live in `apps/quill/templates/`
6. User customizations go in `overrides/quill/`
7. `public/index.php` routes `/quill/*` to Quill's routes
8. Quill uses `core/php/ledger-client.php` to call Ledger's API for billing
9. Quill's `rooster.json` declares its credit cost per operation

Each app is self-contained. Each app can be deployed standalone on its own server or alongside other apps under one `public/index.php` router.

---

## Hosted SaaS Layer

The hosted version of each app is a thin wrapper that adds three things the open source version doesn't have:

- **Signup flow** — new account creation
- **Software billing** — Stripe subscription or usage fee for the hosted service itself (separate from the credits system)
- **Multi-tenancy** — one SQLite database per customer, isolated completely

The tenant router maps incoming requests to the right database:

```php
$tenant = resolve_tenant($_SERVER['HTTP_HOST']); // e.g. acme.ledger.roostermade.com
$db = db_connect("data/tenants/{$tenant}.db");
```

The single-tenant architecture of the open source app makes this clean — no tenant ID columns, no shared tables, no risk of data leakage.

---

## Marketing Sites

Each product has a static marketing site deployed separately:

- Pure HTML, CSS, vanilla JS
- No PHP, no database
- Includes a LocalStorage-based demo that simulates the product without a backend
- Pricing page has two columns: self-hosted (free, GitHub link) and hosted (paid, signup link)
- Cross-promotes other RoosterMade products

---

## Development Notes

**Starting local dev server:**
```bash
php -S localhost:8000 -t public/
```

**Running migrations:**
```bash
php apps/ledger/install/migrate.php
```

**Running the MCP server:**
```bash
php core/mcp/server.php
```

**Running tests:**
```bash
bash tests/run.sh
```

**Seeding local dev data:**
```bash
php apps/ledger/install/seed.php
```

Tests use in-memory SQLite (`:memory:`) and mock all HTTP calls. No real network requests are made during testing. Tests exit with code 0 on pass, code 1 on failure — CI-ready from day one.

---

## Guiding Principles

- **No dependencies** — if it requires a package manager, find another way
- **Boring by default** — product names are literal and descriptive (Ledger, Quill, not Coop or Perch)
- **Elroy writes the changelog** — maintain the narrative in release notes and docs
- **Override, don't fork** — customization happens in `overrides/`, never in `core/`, or `apps/`
- **Prepaid only** — Ledger is not a general billing system; it does one thing well
- **Agents are first-class users** — MCP is not an afterthought; every product ships with it
- **Self-contained apps** — each app can run standalone without the rest of the platform
- **Single tenant** — one installation serves one operator; simplicity is the feature