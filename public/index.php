<?php

// 1. Config (env + session + error reporting)
require_once __DIR__ . '/../apps/ledger/config/config.php';

// 2. Helper: load core file with optional override
function load_with_override(string $app, string $file): void {
    $override = __DIR__ . "/../overrides/{$app}/php/{$file}";
    $core     = __DIR__ . "/../core/php/{$file}";
    if (file_exists($override)) require_once $override;
    if (file_exists($core))     require_once $core;
}

// 3. Load platform core (override-aware)
load_with_override('ledger', 'db.php');
load_with_override('ledger', 'response.php');
load_with_override('ledger', 'auth.php');
load_with_override('ledger', 'email.php');
load_with_override('ledger', 'stripe.php');
load_with_override('ledger', 'webhooks.php');
load_with_override('ledger', 'migrate.php');
load_with_override('ledger', 'router.php');

// 4. Connect to database
$db = core_db_connect(__DIR__ . '/../apps/ledger/data/ledger.db');

// 5. Run migrations
core_migrate_run($db, __DIR__ . '/../apps/ledger/db/migrations');

// 6. Register routes
require_once __DIR__ . '/../apps/ledger/core/php/routes.php';

// 7. Dispatch
core_router_dispatch();
