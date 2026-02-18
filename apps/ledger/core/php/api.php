<?php

// ---------------------------------------------------------------------------
// Exceptions
// ---------------------------------------------------------------------------

class InsufficientFundsException extends RuntimeException {}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

function apps_ledger_api_create_customer(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $body = json_decode(file_get_contents('php://input'), true) ?? [];

    $name       = trim($body['name']  ?? '');
    $email      = trim($body['email'] ?? '');
    $external_id          = $body['external_id']          ?? null;
    $low_balance_threshold = (int) ($body['low_balance_threshold'] ?? 100);

    if ($name === '' || $email === '') {
        core_response_error('name and email are required', 400);
    }

    $now = date('Y-m-d H:i:s');
    $id  = core_db_insert($db,
        'INSERT INTO customers (name, email, external_id, low_balance_threshold, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)',
        [$name, $email, $external_id, $low_balance_threshold, $now, $now]
    );

    $row = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);
    core_response_success($row, 201);
}

function apps_ledger_api_get_customer(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id  = $params['id'];
    $row = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$row) {
        core_response_error('Customer not found', 404);
    }

    core_response_success($row);
}

function apps_ledger_api_get_customer_invoice(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $customer = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$customer) {
        core_response_error('Customer not found', 404);
    }

    $from = $_GET['from'] ?? date('Y-m-01');
    $to   = $_GET['to']   ?? date('Y-m-t');

    $transactions = core_db_all($db,
        'SELECT * FROM transactions WHERE customer_id = ? AND created_at BETWEEN ? AND ? ORDER BY created_at ASC',
        [$id, $from, $to]
    );

    $payments = core_db_all($db,
        "SELECT * FROM payments WHERE customer_id = ? AND created_at BETWEEN ? AND ? AND status = 'completed' ORDER BY created_at ASC",
        [$id, $from, $to]
    );

    $total_debited  = 0;
    $total_credited = 0;
    foreach ($transactions as $tx) {
        if ($tx['type'] === 'debit') {
            $total_debited += (int) $tx['amount'];
        } else {
            $total_credited += (int) $tx['amount'];
        }
    }

    core_response_success([
        'customer'       => $customer,
        'period'         => ['from' => $from, 'to' => $to],
        'transactions'   => $transactions,
        'payments'       => $payments,
        'total_debited'  => $total_debited,
        'total_credited' => $total_credited,
    ]);
}

// ---------------------------------------------------------------------------
// Services
// ---------------------------------------------------------------------------

function apps_ledger_api_list_services(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $rows = core_db_all($db, 'SELECT * FROM services ORDER BY name');
    core_response_success($rows);
}

function apps_ledger_api_create_service(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $body = json_decode(file_get_contents('php://input'), true) ?? [];

    $slug        = trim($body['slug']        ?? '');
    $name        = trim($body['name']        ?? '');
    $description = $body['description']      ?? null;
    $credit_cost = isset($body['credit_cost']) ? (int) $body['credit_cost'] : null;

    if ($slug === '' || $name === '' || $credit_cost === null) {
        core_response_error('slug, name, and credit_cost are required', 400);
    }

    if ($credit_cost < 1) {
        core_response_error('credit_cost must be at least 1', 400);
    }

    $now = date('Y-m-d H:i:s');

    try {
        $id = core_db_insert($db,
            'INSERT INTO services (slug, name, description, credit_cost, created_at, updated_at)
             VALUES (?, ?, ?, ?, ?, ?)',
            [$slug, $name, $description, $credit_cost, $now, $now]
        );
    } catch (PDOException $e) {
        if (str_starts_with($e->getCode(), '23')) {
            core_response_error('Service with that slug already exists', 409);
        }
        throw $e;
    }

    $row = core_db_get($db, 'SELECT * FROM services WHERE id = ?', [$id]);
    core_response_success($row, 201);
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

function apps_ledger_api_record_usage(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $body = json_decode(file_get_contents('php://input'), true) ?? [];

    $customer_id  = $body['customer_id']  ?? null;
    $service_slug = $body['service_slug'] ?? null;
    $units        = isset($body['units']) ? (int) $body['units'] : null;
    $reference_id = $body['reference_id'] ?? null;
    $description  = $body['description']  ?? null;

    if (!$customer_id || !$service_slug || $units === null || !$reference_id || !$description) {
        core_response_error('customer_id, service_slug, units, reference_id, and description are required', 400);
    }

    if ($units < 1) {
        core_response_error('units must be at least 1', 400);
    }

    // Idempotency check
    $existing = core_db_get($db,
        'SELECT * FROM transactions WHERE service_slug = ? AND reference_id = ?',
        [$service_slug, $reference_id]
    );
    if ($existing) {
        core_response_success([
            'transaction'         => $existing,
            'balance_after'       => $existing['balance_after'],
            'low_balance_threshold' => null,
        ]);
    }

    try {
        $result = core_db_transaction($db, function (PDO $db) use ($customer_id, $service_slug, $units, $reference_id, $description) {
            $customer = core_db_get($db, "SELECT * FROM customers WHERE id = ? AND active = 1", [$customer_id]);
            if (!$customer) {
                core_response_error('Customer not found', 404);
            }

            $service = core_db_get($db, "SELECT * FROM services WHERE slug = ? AND active = 1", [$service_slug]);
            if (!$service) {
                core_response_error('Service not found', 404);
            }

            $cost          = $service['credit_cost'] * $units;
            $balance       = (int) $customer['balance'];
            if ($balance < $cost) {
                throw new InsufficientFundsException();
            }

            $balance_after = $balance - $cost;
            $now           = date('Y-m-d H:i:s');

            core_db_run($db,
                'UPDATE customers SET balance = ?, updated_at = ? WHERE id = ?',
                [$balance_after, $now, $customer_id]
            );

            $tx_id = core_db_insert($db,
                'INSERT INTO transactions (customer_id, type, amount, balance_after, service_slug, reference_id, description, created_at)
                 VALUES (?, \'debit\', ?, ?, ?, ?, ?, ?)',
                [$customer_id, $cost, $balance_after, $service_slug, $reference_id, $description, $now]
            );

            $tx = core_db_get($db, 'SELECT * FROM transactions WHERE id = ?', [$tx_id]);

            return [
                'transaction'          => $tx,
                'balance_after'        => $balance_after,
                'low_balance_threshold' => (int) $customer['low_balance_threshold'],
            ];
        });
    } catch (InsufficientFundsException $e) {
        core_response_error('insufficient_funds', 402);
    }

    core_webhooks_dispatch($db, 'usage.recorded', $result);

    $balance_after         = $result['balance_after'];
    $low_balance_threshold = $result['low_balance_threshold'];

    if ($balance_after === 0) {
        core_webhooks_dispatch($db, 'balance.depleted', $result);
    } elseif ($balance_after > 0 && $balance_after < $low_balance_threshold) {
        core_webhooks_dispatch($db, 'balance.low', $result);
    }

    core_response_success($result);
}

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

function apps_ledger_api_list_transactions(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $customer = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$customer) {
        core_response_error('Customer not found', 404);
    }

    $limit  = min((int) ($_GET['limit']  ?? 50), 200);
    $offset = (int) ($_GET['offset'] ?? 0);

    $where  = ['customer_id = ?'];
    $values = [$id];

    if (isset($_GET['service_slug'])) {
        $where[]  = 'service_slug = ?';
        $values[] = $_GET['service_slug'];
    }

    if (isset($_GET['type'])) {
        $where[]  = 'type = ?';
        $values[] = $_GET['type'];
    }

    $sql    = 'SELECT * FROM transactions WHERE ' . implode(' AND ', $where) . ' ORDER BY created_at DESC LIMIT ? OFFSET ?';
    $values[] = $limit;
    $values[] = $offset;

    $rows = core_db_all($db, $sql, $values);

    core_response_success(['transactions' => $rows, 'limit' => $limit, 'offset' => $offset]);
}

function apps_ledger_api_credit_customer(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $customer = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$customer) {
        core_response_error('Customer not found', 404);
    }

    $body        = json_decode(file_get_contents('php://input'), true) ?? [];
    $amount      = isset($body['amount']) ? (int) $body['amount'] : null;
    $description = $body['description'] ?? null;

    if ($amount === null || $amount < 1) {
        core_response_error('amount must be an integer of at least 1', 400);
    }

    $result = core_db_transaction($db, function (PDO $db) use ($id, $amount, $description) {
        $customer      = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);
        $balance_after = (int) $customer['balance'] + $amount;
        $now           = date('Y-m-d H:i:s');

        core_db_run($db,
            'UPDATE customers SET balance = ?, updated_at = ? WHERE id = ?',
            [$balance_after, $now, $id]
        );

        $tx_id = core_db_insert($db,
            'INSERT INTO transactions (customer_id, type, amount, balance_after, service_slug, reference_id, description, created_at)
             VALUES (?, \'credit\', ?, ?, NULL, NULL, ?, ?)',
            [$id, $amount, $balance_after, $description, $now]
        );

        $tx = core_db_get($db, 'SELECT * FROM transactions WHERE id = ?', [$tx_id]);

        return ['transaction' => $tx, 'balance_after' => $balance_after];
    });

    core_webhooks_dispatch($db, 'balance.credited', $result);
    core_response_success($result);
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

function apps_ledger_api_list_payments(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $customer = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$customer) {
        core_response_error('Customer not found', 404);
    }

    $rows = core_db_all($db,
        'SELECT * FROM payments WHERE customer_id = ? ORDER BY created_at DESC',
        [$id]
    );

    core_response_success($rows);
}

// ---------------------------------------------------------------------------
// Checkout
// ---------------------------------------------------------------------------

function apps_ledger_api_create_checkout(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $customer = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$id]);

    if (!$customer) {
        core_response_error('Customer not found', 404);
    }

    $body         = json_decode(file_get_contents('php://input'), true) ?? [];
    $amount_cents = isset($body['amount_cents']) ? (int) $body['amount_cents'] : null;
    $credits      = isset($body['credits'])      ? (int) $body['credits']      : null;

    if ($amount_cents === null || $amount_cents < 1) {
        core_response_error('amount_cents must be an integer of at least 1', 400);
    }

    if ($credits === null || $credits < 1) {
        core_response_error('credits must be an integer of at least 1', 400);
    }

    // Ensure Stripe customer exists
    if (empty($customer['stripe_customer_id'])) {
        $stripe_customer = core_stripe_create_customer($customer['email'], $customer['name']);

        if (isset($stripe_customer['error'])) {
            core_response_error('Failed to create Stripe customer', 502);
        }

        $now = date('Y-m-d H:i:s');
        core_db_run($db,
            'UPDATE customers SET stripe_customer_id = ?, updated_at = ? WHERE id = ?',
            [$stripe_customer['id'], $now, $id]
        );
        $customer['stripe_customer_id'] = $stripe_customer['id'];
    }

    $success_url = config('APP_URL') . '/payment/success';
    $cancel_url  = config('APP_URL') . '/payment/cancel';

    $session = core_stripe_create_checkout_session(
        $customer['stripe_customer_id'],
        $amount_cents,
        $credits,
        $success_url,
        $cancel_url
    );

    if (isset($session['error'])) {
        core_response_error('Failed to create checkout session', 502);
    }

    $now = date('Y-m-d H:i:s');
    core_db_insert($db,
        'INSERT INTO payments (customer_id, stripe_session_id, stripe_payment_id, amount_cents, credits_granted, status, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, \'pending\', ?, ?)',
        [
            $id,
            $session['id'],
            $session['payment_intent'] ?? $session['id'],
            $amount_cents,
            $credits,
            $now,
            $now,
        ]
    );

    core_response_success(['url' => $session['url']]);
}

// ---------------------------------------------------------------------------
// Stripe Webhook
// ---------------------------------------------------------------------------

function apps_ledger_api_handle_stripe_webhook(array $params): void
{
    global $db;

    $payload = file_get_contents('php://input');
    $sig     = $_SERVER['HTTP_STRIPE_SIGNATURE'] ?? '';
    $event   = core_stripe_verify_webhook($payload, $sig);

    if ($event === false) {
        core_response_error('Invalid webhook signature', 400);
    }

    if ($event['type'] === 'checkout.session.completed') {
        $session = $event['data']['object'];

        $payment = core_db_get($db,
            'SELECT * FROM payments WHERE stripe_session_id = ?',
            [$session['id']]
        );

        if (!$payment) {
            core_response_success(null);
        }

        if ($payment['status'] === 'completed') {
            core_response_success(null);
        }

        $credits = (int) ($session['metadata']['credits'] ?? 0);

        core_db_transaction($db, function (PDO $db) use ($payment, $session, $credits) {
            $customer      = core_db_get($db, 'SELECT * FROM customers WHERE id = ?', [$payment['customer_id']]);
            $balance_after = (int) $customer['balance'] + $credits;
            $now           = date('Y-m-d H:i:s');

            core_db_run($db,
                'UPDATE customers SET balance = ?, updated_at = ? WHERE id = ?',
                [$balance_after, $now, $payment['customer_id']]
            );

            core_db_run($db,
                "UPDATE payments SET status = 'completed', stripe_payment_id = ?, updated_at = ? WHERE id = ?",
                [$session['payment_intent'] ?? $session['id'], $now, $payment['id']]
            );

            core_db_insert($db,
                'INSERT INTO transactions (customer_id, type, amount, balance_after, service_slug, description, created_at)
                 VALUES (?, \'credit\', ?, ?, NULL, \'Credit purchase via Stripe\', ?)',
                [$payment['customer_id'], $credits, $balance_after, $now]
            );
        });

        core_webhooks_dispatch($db, 'payment.received', [
            'payment_id'  => $payment['id'],
            'customer_id' => $payment['customer_id'],
            'credits'     => $credits,
        ]);
    }

    core_response_success(null);
}

// ---------------------------------------------------------------------------
// Webhook Endpoints
// ---------------------------------------------------------------------------

function apps_ledger_api_register_webhook_endpoint(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $body   = json_decode(file_get_contents('php://input'), true) ?? [];
    $url    = trim($body['url']    ?? '');
    $events = $body['events']      ?? null;
    $label  = $body['label']       ?? null;
    $secret = $body['secret']      ?? bin2hex(random_bytes(32));

    if ($url === '') {
        core_response_error('url is required', 400);
    }

    if (!is_array($events) || count($events) === 0) {
        core_response_error('events must be a non-empty array', 400);
    }

    $now = date('Y-m-d H:i:s');
    $id  = core_db_insert($db,
        'INSERT INTO webhook_endpoints (url, events, label, secret, active, created_at, updated_at)
         VALUES (?, ?, ?, ?, 1, ?, ?)',
        [$url, json_encode($events), $label, $secret, $now, $now]
    );

    $row = core_db_get($db, 'SELECT * FROM webhook_endpoints WHERE id = ?', [$id]);
    core_response_success($row, 201);
}

function apps_ledger_api_delete_webhook_endpoint(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id  = $params['id'];
    $now = date('Y-m-d H:i:s');

    $stmt = $db->prepare('UPDATE webhook_endpoints SET active = 0, updated_at = ? WHERE id = ?');
    $stmt->execute([$now, $id]);

    if ($stmt->rowCount() === 0) {
        core_response_error('Webhook endpoint not found', 404);
    }

    core_response_success(null);
}

function apps_ledger_api_list_webhook_deliveries(array $params): void
{
    core_auth_require_api_key();
    global $db;

    $id       = $params['id'];
    $endpoint = core_db_get($db, 'SELECT * FROM webhook_endpoints WHERE id = ?', [$id]);

    if (!$endpoint) {
        core_response_error('Webhook endpoint not found', 404);
    }

    $rows = core_db_all($db,
        'SELECT * FROM webhook_deliveries WHERE webhook_endpoint_id = ? ORDER BY created_at DESC LIMIT 100',
        [$id]
    );

    core_response_success($rows);
}
