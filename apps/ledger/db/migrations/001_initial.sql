PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- schema_versions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS schema_versions (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- admin_users
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS admin_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT    NOT NULL UNIQUE,
    name          TEXT,
    active        INTEGER NOT NULL DEFAULT 1,
    last_login_at TEXT,
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- magic_codes
-- One-time login codes sent to admin users. 6-digit numeric strings.
-- Stored hashed (HMAC-SHA256). Expire after 15 minutes.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS magic_codes (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_user_id INTEGER NOT NULL REFERENCES admin_users(id),
    code          TEXT    NOT NULL,
    expires_at    TEXT    NOT NULL,
    used          INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_magic_codes_admin_user_id ON magic_codes(admin_user_id);

-- ---------------------------------------------------------------------------
-- services
-- Registered micro-services that consume credits.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS services (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    slug        TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    description TEXT,
    credit_cost INTEGER NOT NULL DEFAULT 1,
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- customers
-- End-users with a unified prepaid credit balance.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS customers (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id           TEXT    UNIQUE,
    name                  TEXT    NOT NULL,
    email                 TEXT    NOT NULL UNIQUE,
    stripe_customer_id    TEXT    UNIQUE,
    balance               INTEGER NOT NULL DEFAULT 0,
    low_balance_threshold INTEGER NOT NULL DEFAULT 100,
    active                INTEGER NOT NULL DEFAULT 1,
    created_at            TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- transactions
-- Every credit and debit with a balance snapshot.
-- (service_slug, reference_id) is unique for idempotency on debits.
-- NULL values in either column do not participate in the unique check,
-- so manual credits (no service_slug/reference_id) are unrestricted.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS transactions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id   INTEGER NOT NULL REFERENCES customers(id),
    service_slug  TEXT,
    type          TEXT    NOT NULL CHECK (type IN ('credit', 'debit')),
    amount        INTEGER NOT NULL CHECK (amount > 0),
    balance_after INTEGER NOT NULL,
    description   TEXT,
    reference_id  TEXT,
    meta          TEXT,
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),

    UNIQUE (service_slug, reference_id)
);

CREATE INDEX IF NOT EXISTS idx_transactions_customer_id  ON transactions(customer_id);
CREATE INDEX IF NOT EXISTS idx_transactions_service_slug ON transactions(service_slug);

-- ---------------------------------------------------------------------------
-- payments
-- Stripe payment records. Source of truth for credit purchases.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS payments (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id       INTEGER NOT NULL REFERENCES customers(id),
    stripe_payment_id TEXT    NOT NULL UNIQUE,
    stripe_session_id TEXT    UNIQUE,
    amount_cents      INTEGER NOT NULL,
    credits_granted   INTEGER NOT NULL,
    status            TEXT    NOT NULL DEFAULT 'pending',
    created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_payments_customer_id ON payments(customer_id);

-- ---------------------------------------------------------------------------
-- webhook_endpoints
-- Registered outbound webhook URLs.
-- events is a JSON array of subscribed event type strings.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT,
    url        TEXT NOT NULL,
    secret     TEXT NOT NULL,
    events     TEXT NOT NULL,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ---------------------------------------------------------------------------
-- webhook_deliveries
-- Delivery log with retry state. Retried up to 5 times with exponential backoff.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    webhook_endpoint_id INTEGER NOT NULL REFERENCES webhook_endpoints(id),
    event               TEXT    NOT NULL,
    payload             TEXT    NOT NULL,
    status              TEXT    NOT NULL DEFAULT 'pending',
    attempts            INTEGER NOT NULL DEFAULT 0,
    last_attempted_at   TEXT,
    next_retry_at       TEXT,
    response_code       INTEGER,
    created_at          TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint_id   ON webhook_deliveries(webhook_endpoint_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status_retry  ON webhook_deliveries(status, next_retry_at);

-- ---------------------------------------------------------------------------
-- Mark this migration as applied
-- ---------------------------------------------------------------------------
INSERT OR IGNORE INTO schema_versions (version, applied_at) VALUES (1, datetime('now'));
