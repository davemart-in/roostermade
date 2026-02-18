<?php

/**
 * core/php/stripe.php
 *
 * Raw Stripe API helpers for RoosterMade apps.
 * No SDK. No Composer. Uses file_get_contents + stream_context_create.
 *
 * Transport:  GET → query string, POST → application/x-www-form-urlencoded
 * API version: 2023-10-16 (pinned)
 * Error model: functions always return decoded JSON. Callers check for ['error'] key.
 *
 * All functions wrapped in if (!function_exists()) for overridability.
 */

if (!function_exists('core_stripe_request')) {
    /**
     * Make a raw request to the Stripe API.
     *
     * @param string $method   'GET' or 'POST'
     * @param string $endpoint API endpoint, e.g. 'customers', 'checkout/sessions'
     * @param array  $data     Request parameters (query string for GET, form body for POST)
     * @return array Decoded JSON response. Contains ['error'] key on failure.
     */
    function core_stripe_request(string $method, string $endpoint, array $data = []): array
    {
        $url     = 'https://api.stripe.com/v1/' . $endpoint;
        $api_key = getenv('STRIPE_SECRET_KEY') ?: '';
        $method  = strtoupper($method);

        $base_headers = [
            'Authorization: Bearer ' . $api_key,
            'Stripe-Version: 2023-10-16',
        ];

        if ($method === 'GET') {
            if ($data) {
                $url .= '?' . http_build_query($data);
            }
            $context = stream_context_create([
                'http' => [
                    'method'        => 'GET',
                    'header'        => implode("\r\n", $base_headers),
                    'ignore_errors' => true,
                    'timeout'       => 30,
                ],
            ]);
        } else {
            $body    = http_build_query($data);
            $headers = array_merge($base_headers, [
                'Content-Type: application/x-www-form-urlencoded',
                'Content-Length: ' . strlen($body),
            ]);
            $context = stream_context_create([
                'http' => [
                    'method'        => 'POST',
                    'header'        => implode("\r\n", $headers),
                    'content'       => $body,
                    'ignore_errors' => true,
                    'timeout'       => 30,
                ],
            ]);
        }

        $response = @file_get_contents($url, false, $context);

        if ($response === false) {
            return ['error' => ['message' => 'Stripe request failed: no response']];
        }

        $decoded = json_decode($response, true);

        if (!is_array($decoded)) {
            return ['error' => ['message' => 'Stripe request failed: invalid JSON response']];
        }

        return $decoded;
    }
}

if (!function_exists('core_stripe_create_customer')) {
    /**
     * Create a Stripe customer.
     *
     * @param string $email Customer email address.
     * @param string $name  Customer display name.
     * @return array Stripe customer object, or ['error'] on failure.
     */
    function core_stripe_create_customer(string $email, string $name): array
    {
        return core_stripe_request('POST', 'customers', [
            'email' => $email,
            'name'  => $name,
        ]);
    }
}

if (!function_exists('core_stripe_create_checkout_session')) {
    /**
     * Create a Stripe Checkout Session for a one-time credit purchase.
     *
     * The number of credits is stored in session metadata so the webhook
     * handler can read it from $event['data']['object']['metadata']['credits'].
     *
     * @param string $stripe_customer_id Stripe customer ID (cus_...)
     * @param int    $amount_cents       Price in cents (e.g. 1000 = $10.00 USD)
     * @param int    $credits            Number of credits the customer will receive
     * @param string $success_url        URL to redirect to after successful payment
     * @param string $cancel_url         URL to redirect to if customer cancels
     * @return array Stripe checkout session object, or ['error'] on failure.
     */
    function core_stripe_create_checkout_session(
        string $stripe_customer_id,
        int    $amount_cents,
        int    $credits,
        string $success_url,
        string $cancel_url
    ): array {
        return core_stripe_request('POST', 'checkout/sessions', [
            'customer'                                          => $stripe_customer_id,
            'mode'                                             => 'payment',
            'line_items'                                       => [[
                'price_data' => [
                    'currency'     => 'usd',
                    'unit_amount'  => $amount_cents,
                    'product_data' => [
                        'name' => $credits . ' Credits',
                    ],
                ],
                'quantity' => 1,
            ]],
            'metadata'    => ['credits' => $credits],
            'success_url' => $success_url,
            'cancel_url'  => $cancel_url,
        ]);
    }
}

if (!function_exists('core_stripe_verify_webhook')) {
    /**
     * Verify a Stripe webhook signature and return the decoded event.
     *
     * Implements Stripe's standard verification:
     * - Parses t= and v1= from the Stripe-Signature header
     * - Rejects events older than 300 seconds (replay protection)
     * - Verifies HMAC-SHA256 signature using STRIPE_WEBHOOK_SECRET
     *
     * @param string $payload    Raw request body (do not JSON-decode before passing)
     * @param string $sig_header Value of the Stripe-Signature HTTP header
     * @return array|false Decoded event array on success, false on any verification failure
     */
    function core_stripe_verify_webhook(string $payload, string $sig_header): array|false
    {
        // Parse "t=1234,v1=abc,v1=def" into components
        $parts     = [];
        $timestamp = null;
        $v1        = null;

        foreach (explode(',', $sig_header) as $part) {
            [$key, $value] = array_pad(explode('=', $part, 2), 2, '');
            $key = trim($key);
            if ($key === 't') {
                $timestamp = (int) $value;
            } elseif ($key === 'v1' && $v1 === null) {
                // Use first v1 value (Stripe may send multiple for key rotation)
                $v1 = trim($value);
            }
        }

        if ($timestamp === null || $v1 === null || $v1 === '') {
            return false;
        }

        // Reject stale events
        if (abs(time() - $timestamp) > 300) {
            return false;
        }

        $secret        = getenv('STRIPE_WEBHOOK_SECRET') ?: '';
        $signed_string = $timestamp . '.' . $payload;
        $expected      = hash_hmac('sha256', $signed_string, $secret);

        if (!hash_equals($expected, $v1)) {
            return false;
        }

        $event = json_decode($payload, true);

        return is_array($event) ? $event : false;
    }
}
