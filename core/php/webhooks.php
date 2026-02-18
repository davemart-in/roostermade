<?php

/**
 * core/php/webhooks.php
 *
 * Outbound webhook dispatch and retry for RoosterMade apps.
 *
 * Wire format: JSON envelope {"event":"...","timestamp":"...","data":{...}}
 * Signature:   X-RoosterMade-Signature: sha256=<hmac_hex> (HMAC over raw envelope JSON)
 * Backoff:     5 → 15 → 60 → 180 → 360 minutes. Max 5 attempts total.
 *
 * Public functions are wrapped in if (!function_exists()) for overridability.
 * Internal helpers (_core_webhooks_*) are not wrapped and not overridable.
 */

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Attempt a single webhook delivery.
 * Signs the body with HMAC-SHA256 and POSTs to the endpoint URL.
 *
 * @param array  $endpoint  Row from webhook_endpoints.
 * @param string $body      Raw JSON envelope to deliver.
 * @return array{success: bool, code: int}
 */
function _core_webhooks_attempt_delivery(array $endpoint, string $body): array
{
    $signature = 'sha256=' . hash_hmac('sha256', $body, $endpoint['secret']);

    $context = stream_context_create([
        'http' => [
            'method'        => 'POST',
            'header'        => implode("\r\n", [
                'Content-Type: application/json',
                'X-RoosterMade-Signature: ' . $signature,
                'User-Agent: RoosterMade-Webhooks/1.0',
            ]),
            'content'       => $body,
            'ignore_errors' => true,
            'timeout'       => 10,
        ],
    ]);

    $response = @file_get_contents($endpoint['url'], false, $context);
    $code     = 0;

    if (!empty($http_response_header)) {
        preg_match('/HTTP\/\S+\s+(\d+)/', $http_response_header[0], $m);
        $code = isset($m[1]) ? (int) $m[1] : 0;
    }

    return [
        'success' => $code >= 200 && $code < 300,
        'code'    => $code,
    ];
}

/**
 * Calculate the next retry timestamp based on how many attempts have been made.
 *
 * Backoff offsets (minutes after the Nth failure):
 *   1 → 5, 2 → 15, 3 → 60, 4 → 180, 5+ → 360
 *
 * @param int $attempts Number of attempts just completed.
 * @return string SQLite-compatible datetime string.
 */
function _core_webhooks_next_retry_at(int $attempts): string
{
    $offsets = [1 => 5, 2 => 15, 3 => 60, 4 => 180];
    $minutes = $offsets[$attempts] ?? 360;
    return date('Y-m-d H:i:s', time() + $minutes * 60);
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

if (!function_exists('core_webhooks_dispatch')) {
    /**
     * Dispatch a webhook event to all subscribed active endpoints.
     *
     * Creates a delivery record for each matching endpoint, attempts delivery
     * immediately, and records the result. Failed deliveries are scheduled
     * for retry by core_webhooks_retry_failed().
     *
     * Note: delivery is synchronous. Slow endpoints add latency to the caller.
     *
     * @param PDO    $db      Active database connection.
     * @param string $event   Event name, e.g. 'usage.recorded'.
     * @param array  $payload Event data to include in the envelope.
     * @return void
     */
    function core_webhooks_dispatch(PDO $db, string $event, array $payload): void
    {
        $endpoints = core_db_all($db, 'SELECT * FROM webhook_endpoints WHERE active = 1');

        $envelope = json_encode([
            'event'     => $event,
            'timestamp' => gmdate('c'),
            'data'      => $payload,
        ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);

        $now = date('Y-m-d H:i:s');

        foreach ($endpoints as $endpoint) {
            $subscribed = json_decode($endpoint['events'], true) ?? [];
            if (!in_array($event, $subscribed, true)) {
                continue;
            }

            // Create delivery record before attempting so we have a record even on exception
            $delivery_id = core_db_insert($db,
                'INSERT INTO webhook_deliveries
                    (webhook_endpoint_id, event, payload, status, attempts, created_at)
                 VALUES (?, ?, ?, \'pending\', 0, ?)',
                [$endpoint['id'], $event, $envelope, $now]
            );

            $result = _core_webhooks_attempt_delivery($endpoint, $envelope);

            if ($result['success']) {
                core_db_run($db,
                    'UPDATE webhook_deliveries
                     SET status = \'delivered\', attempts = 1,
                         response_code = ?, last_attempted_at = ?, next_retry_at = NULL
                     WHERE id = ?',
                    [$result['code'], $now, $delivery_id]
                );
            } else {
                core_db_run($db,
                    'UPDATE webhook_deliveries
                     SET status = \'failed\', attempts = 1,
                         response_code = ?, last_attempted_at = ?, next_retry_at = ?
                     WHERE id = ?',
                    [$result['code'], $now, _core_webhooks_next_retry_at(1), $delivery_id]
                );
            }
        }
    }
}

if (!function_exists('core_webhooks_retry_failed')) {
    /**
     * Retry all failed webhook deliveries that are due.
     *
     * Finds deliveries where status='failed', next_retry_at <= now, and attempts < 5.
     * Designed to be called by a cron job or an admin-triggered endpoint.
     *
     * @param PDO $db Active database connection.
     * @return void
     */
    function core_webhooks_retry_failed(PDO $db): void
    {
        $deliveries = core_db_all($db,
            "SELECT * FROM webhook_deliveries
             WHERE status = 'failed'
               AND next_retry_at <= datetime('now')
               AND attempts < 5"
        );

        $now = date('Y-m-d H:i:s');

        foreach ($deliveries as $delivery) {
            $endpoint = core_db_get($db,
                'SELECT * FROM webhook_endpoints WHERE id = ?',
                [$delivery['webhook_endpoint_id']]
            );

            // Skip if endpoint was deleted or deactivated
            if (!$endpoint || !$endpoint['active']) {
                continue;
            }

            $result       = _core_webhooks_attempt_delivery($endpoint, $delivery['payload']);
            $new_attempts = (int) $delivery['attempts'] + 1;

            if ($result['success']) {
                core_db_run($db,
                    'UPDATE webhook_deliveries
                     SET status = \'delivered\', attempts = ?,
                         response_code = ?, last_attempted_at = ?, next_retry_at = NULL
                     WHERE id = ?',
                    [$new_attempts, $result['code'], $now, $delivery['id']]
                );
            } elseif ($new_attempts >= 5) {
                // Final attempt exhausted — give up, no further retries
                core_db_run($db,
                    'UPDATE webhook_deliveries
                     SET status = \'failed\', attempts = ?,
                         response_code = ?, last_attempted_at = ?, next_retry_at = NULL
                     WHERE id = ?',
                    [$new_attempts, $result['code'], $now, $delivery['id']]
                );
            } else {
                core_db_run($db,
                    'UPDATE webhook_deliveries
                     SET status = \'failed\', attempts = ?,
                         response_code = ?, last_attempted_at = ?, next_retry_at = ?
                     WHERE id = ?',
                    [$new_attempts, $result['code'], $now,
                     _core_webhooks_next_retry_at($new_attempts), $delivery['id']]
                );
            }
        }
    }
}
