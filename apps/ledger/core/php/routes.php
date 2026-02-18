<?php

require_once __DIR__ . '/api.php';

// Customers
core_router_route('POST',   '/api/ledger/v1/customers',                   'apps_ledger_api_create_customer');
core_router_route('GET',    '/api/ledger/v1/customers/{id}',              'apps_ledger_api_get_customer');
core_router_route('GET',    '/api/ledger/v1/customers/{id}/invoice',      'apps_ledger_api_get_customer_invoice');

// Services
core_router_route('GET',    '/api/ledger/v1/services',                    'apps_ledger_api_list_services');
core_router_route('POST',   '/api/ledger/v1/services',                    'apps_ledger_api_create_service');

// Usage
core_router_route('POST',   '/api/ledger/v1/usage',                       'apps_ledger_api_record_usage');

// Transactions
core_router_route('GET',    '/api/ledger/v1/customers/{id}/transactions', 'apps_ledger_api_list_transactions');
core_router_route('POST',   '/api/ledger/v1/customers/{id}/credit',       'apps_ledger_api_credit_customer');

// Payments
core_router_route('GET',    '/api/ledger/v1/customers/{id}/payments',     'apps_ledger_api_list_payments');

// Checkout
core_router_route('POST',   '/api/ledger/v1/customers/{id}/checkout',     'apps_ledger_api_create_checkout');

// Stripe webhook (no API key auth)
core_router_route('POST',   '/api/ledger/v1/stripe/webhook',              'apps_ledger_api_handle_stripe_webhook');

// Webhook management
core_router_route('POST',   '/api/ledger/v1/webhooks',                    'apps_ledger_api_register_webhook_endpoint');
core_router_route('DELETE', '/api/ledger/v1/webhooks/{id}',               'apps_ledger_api_delete_webhook_endpoint');
core_router_route('GET',    '/api/ledger/v1/webhooks/{id}/deliveries',    'apps_ledger_api_list_webhook_deliveries');
