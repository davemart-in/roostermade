<?php

// ── .env loader ───────────────────────────────────────────────────────────────
$envFile = __DIR__ . '/.env';
if (file_exists($envFile)) {
    $lines = file($envFile, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
    foreach ($lines as $line) {
        if (str_starts_with(ltrim($line), '#')) continue;
        $pos = strpos($line, '=');
        if ($pos === false) continue;
        $key   = trim(substr($line, 0, $pos));
        $value = trim(substr($line, $pos + 1));
        // Strip surrounding single or double quotes
        if (
            (str_starts_with($value, '"') && str_ends_with($value, '"')) ||
            (str_starts_with($value, "'") && str_ends_with($value, "'"))
        ) {
            $value = substr($value, 1, -1);
        }
        // Existing env vars are not overwritten
        if (getenv($key) === false && !isset($_ENV[$key])) {
            putenv("{$key}={$value}");
            $_ENV[$key] = $value;
        }
    }
}

// ── config() helper ───────────────────────────────────────────────────────────
function config(string $key, mixed $default = null): mixed {
    return $_ENV[$key] ?? getenv($key) ?: $default;
}

// ── Error reporting ───────────────────────────────────────────────────────────
if (config('APP_ENV') === 'development') {
    error_reporting(E_ALL);
    ini_set('display_errors', '1');
} else {
    error_reporting(0);
    ini_set('display_errors', '0');
}

// ── Session with secure defaults ──────────────────────────────────────────────
ini_set('session.cookie_httponly', '1');
ini_set('session.cookie_samesite', 'Lax');
ini_set('session.use_strict_mode', '1');
ini_set('session.cookie_secure',   config('APP_ENV') === 'production' ? '1' : '0');
session_start();
