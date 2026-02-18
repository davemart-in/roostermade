<?php

/**
 * core/php/response.php
 *
 * HTTP response helpers for RoosterMade apps.
 * All functions are wrapped in if (!function_exists()) so app-level
 * overrides in overrides/{app}/php/response.php take precedence.
 */

if (!function_exists('core_response_json')) {
    /**
     * Send a JSON response and exit.
     *
     * @param mixed $data   Any JSON-serialisable value.
     * @param int   $status HTTP status code.
     * @return never
     */
    function core_response_json(mixed $data, int $status = 200): never
    {
        http_response_code($status);
        header('Content-Type: application/json');
        echo json_encode($data, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
        exit;
    }
}

if (!function_exists('core_response_error')) {
    /**
     * Send a JSON error response and exit.
     *
     * @param string $message Human-readable error description.
     * @param int    $status  HTTP status code (default 400).
     * @return never
     */
    function core_response_error(string $message, int $status = 400): never
    {
        core_response_json(['error' => $message], $status);
    }
}

if (!function_exists('core_response_success')) {
    /**
     * Send a JSON success response and exit.
     *
     * @param mixed $data   Payload to include under the 'data' key.
     * @param int   $status HTTP status code (default 200).
     * @return never
     */
    function core_response_success(mixed $data = null, int $status = 200): never
    {
        core_response_json(['success' => true, 'data' => $data], $status);
    }
}
