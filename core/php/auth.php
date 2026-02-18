<?php

/**
 * core/php/auth.php
 *
 * Authentication helpers for RoosterMade apps.
 * Covers API key auth (machine-to-machine) and admin session auth (UI).
 * All functions are wrapped in if (!function_exists()) so app-level
 * overrides in overrides/{app}/php/auth.php take precedence.
 */

if (!function_exists('core_auth_generate_magic_code')) {
    /**
     * Generate a random 6-digit numeric code, zero-padded.
     * Suitable for magic link login flows.
     *
     * @return string e.g. "042831"
     */
    function core_auth_generate_magic_code(): string
    {
        return sprintf('%06d', random_int(0, 999999));
    }
}

if (!function_exists('core_auth_verify_api_key')) {
    /**
     * Timing-safe comparison of the provided key against the API_KEY env var.
     *
     * @param string $key The key supplied by the caller.
     * @return bool
     */
    function core_auth_verify_api_key(string $key): bool
    {
        $expected = getenv('API_KEY');

        if ($expected === false || $expected === '') {
            return false;
        }

        return hash_equals($expected, $key);
    }
}

if (!function_exists('core_auth_require_api_key')) {
    /**
     * Enforce API key authentication on the current request.
     * Reads the Authorization: Bearer header and calls core_response_error(401)
     * if the key is missing or invalid.
     *
     * @return void
     */
    function core_auth_require_api_key(): void
    {
        $header = $_SERVER['HTTP_AUTHORIZATION'] ?? '';
        $key    = str_starts_with($header, 'Bearer ') ? substr($header, 7) : '';

        if (!core_auth_verify_api_key($key)) {
            core_response_error('Unauthorized', 401);
        }
    }
}

if (!function_exists('core_auth_require_admin_session')) {
    /**
     * Enforce an active admin session on the current request.
     * Starts the session if not already started.
     * Redirects to /admin/login and exits if no valid session exists.
     *
     * @return void
     */
    function core_auth_require_admin_session(): void
    {
        if (session_status() === PHP_SESSION_NONE) {
            session_start();
        }

        if (empty($_SESSION['admin_user_id'])) {
            header('Location: /admin/login');
            exit;
        }
    }
}

if (!function_exists('core_auth_create_admin_session')) {
    /**
     * Establish an admin session for the given user ID.
     * Regenerates the session ID to prevent fixation attacks.
     *
     * @param int $admin_user_id
     * @return void
     */
    function core_auth_create_admin_session(int $admin_user_id): void
    {
        if (session_status() === PHP_SESSION_NONE) {
            session_start();
        }

        session_regenerate_id(true);
        $_SESSION['admin_user_id'] = $admin_user_id;
    }
}

if (!function_exists('core_auth_destroy_admin_session')) {
    /**
     * Fully destroy the current admin session.
     * Clears session data, invalidates the session cookie, and calls session_destroy().
     *
     * @return void
     */
    function core_auth_destroy_admin_session(): void
    {
        if (session_status() === PHP_SESSION_NONE) {
            session_start();
        }

        $_SESSION = [];

        if (ini_get('session.use_cookies')) {
            $params = session_get_cookie_params();
            setcookie(
                session_name(),
                '',
                time() - 42000,
                $params['path'],
                $params['domain'],
                $params['secure'],
                $params['httponly']
            );
        }

        session_destroy();
    }
}
