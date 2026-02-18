<?php

/**
 * core/php/email.php
 *
 * Email sending helpers for RoosterMade apps.
 * Routes to the correct provider based on the MAIL_PROVIDER env var.
 * Supported providers: resend, mailgun, postmark.
 * Falls back to PHP mail() if no provider is configured.
 *
 * All public and provider functions are wrapped in if (!function_exists())
 * so app-level overrides in overrides/{app}/php/email.php take precedence.
 *
 * _core_email_http_post() is a truly internal helper and is NOT overridable.
 */

// ---------------------------------------------------------------------------
// Internal HTTP helper — not wrapped, not overridable
// ---------------------------------------------------------------------------

/**
 * POST to a URL and return the HTTP status code and response body.
 * Uses file_get_contents + stream_context_create per platform constraints.
 *
 * @param string   $url
 * @param string[] $headers  Array of raw header strings, e.g. ['Content-Type: application/json']
 * @param string   $body     Raw request body.
 * @return array{code: int, body: string}
 */
function _core_email_http_post(string $url, array $headers, string $body): array
{
    $context = stream_context_create([
        'http' => [
            'method'        => 'POST',
            'header'        => implode("\r\n", $headers),
            'content'       => $body,
            'ignore_errors' => true,   // return body even on 4xx/5xx
            'timeout'       => 10,
        ],
    ]);

    $response = @file_get_contents($url, false, $context);
    $response = $response !== false ? $response : '';

    // $http_response_header is set by file_get_contents in the local scope
    $code = 0;
    if (!empty($http_response_header)) {
        preg_match('/HTTP\/\S+\s+(\d+)/', $http_response_header[0], $m);
        $code = isset($m[1]) ? (int) $m[1] : 0;
    }

    return ['code' => $code, 'body' => $response];
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

if (!function_exists('core_email_send')) {
    /**
     * Send an email via the configured provider.
     *
     * @param string      $to        Recipient address.
     * @param string      $subject   Email subject.
     * @param string      $html_body HTML message body.
     * @param string      $text_body Plain-text fallback body (recommended for deliverability).
     * @param string|null $from      Sender address. Defaults to MAIL_FROM env var.
     * @return bool True on success, false on failure.
     */
    function core_email_send(
        string $to,
        string $subject,
        string $html_body,
        string $text_body = '',
        string $from = null
    ): bool {
        $from_email = $from ?? getenv('MAIL_FROM') ?: '';
        $from_name  = getenv('MAIL_FROM_NAME') ?: '';
        $provider   = strtolower(getenv('MAIL_PROVIDER') ?: '');

        return match ($provider) {
            'resend'   => core_email_send_resend($to, $subject, $html_body, $text_body, $from_email, $from_name),
            'mailgun'  => core_email_send_mailgun($to, $subject, $html_body, $text_body, $from_email, $from_name),
            'postmark' => core_email_send_postmark($to, $subject, $html_body, $text_body, $from_email, $from_name),
            default    => core_email_send_mail($to, $subject, $html_body, $text_body, $from_email, $from_name),
        };
    }
}

if (!function_exists('core_email_render')) {
    /**
     * Load an email template and substitute {{variable}} placeholders.
     *
     * Template files are resolved relative to core/templates/:
     *   core_email_render('email/base', [...]) → core/templates/email/base.html
     *
     * @param string               $template Template name, e.g. 'email/base'.
     * @param array<string, mixed> $vars     Key-value pairs for substitution.
     * @return string Rendered HTML, or empty string if template not found.
     */
    function core_email_render(string $template, array $vars): string
    {
        $path = dirname(__FILE__, 2) . '/templates/' . $template . '.html';

        if (!file_exists($path)) {
            trigger_error("core_email_render: template not found: {$path}", E_USER_WARNING);
            return '';
        }

        $html = file_get_contents($path);

        foreach ($vars as $key => $value) {
            $html = str_replace('{{' . $key . '}}', (string) $value, $html);
        }

        return $html;
    }
}

// ---------------------------------------------------------------------------
// Provider helpers
// ---------------------------------------------------------------------------

if (!function_exists('core_email_send_resend')) {
    /**
     * Send via Resend (https://resend.com).
     *
     * Env vars: MAIL_API_KEY
     *
     * @internal Called by core_email_send(). Override to customise Resend behaviour.
     */
    function core_email_send_resend(
        string $to,
        string $subject,
        string $html_body,
        string $text_body,
        string $from_email,
        string $from_name
    ): bool {
        $from    = $from_name ? "{$from_name} <{$from_email}>" : $from_email;
        $api_key = getenv('MAIL_API_KEY') ?: '';

        $payload = json_encode([
            'from'    => $from,
            'to'      => [$to],
            'subject' => $subject,
            'html'    => $html_body,
            'text'    => $text_body,
        ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);

        $result = _core_email_http_post(
            'https://api.resend.com/emails',
            [
                'Content-Type: application/json',
                'Authorization: Bearer ' . $api_key,
            ],
            $payload
        );

        return $result['code'] >= 200 && $result['code'] < 300;
    }
}

if (!function_exists('core_email_send_mailgun')) {
    /**
     * Send via Mailgun (https://mailgun.com).
     *
     * Env vars: MAIL_API_KEY, MAIL_FROM (domain extracted from address),
     *           MAIL_DOMAIN (optional override for Mailgun domain)
     *
     * @internal Called by core_email_send(). Override to customise Mailgun behaviour.
     */
    function core_email_send_mailgun(
        string $to,
        string $subject,
        string $html_body,
        string $text_body,
        string $from_email,
        string $from_name
    ): bool {
        $from    = $from_name ? "{$from_name} <{$from_email}>" : $from_email;
        $api_key = getenv('MAIL_API_KEY') ?: '';

        // Domain: explicit env var wins, otherwise parse from sender address
        $domain = getenv('MAIL_DOMAIN') ?: '';
        if (!$domain) {
            $parts  = explode('@', $from_email, 2);
            $domain = $parts[1] ?? '';
        }

        $fields = http_build_query([
            'from'    => $from,
            'to'      => $to,
            'subject' => $subject,
            'html'    => $html_body,
            'text'    => $text_body,
        ]);

        $result = _core_email_http_post(
            "https://api.mailgun.net/v3/{$domain}/messages",
            [
                'Content-Type: application/x-www-form-urlencoded',
                'Authorization: Basic ' . base64_encode('api:' . $api_key),
            ],
            $fields
        );

        return $result['code'] >= 200 && $result['code'] < 300;
    }
}

if (!function_exists('core_email_send_postmark')) {
    /**
     * Send via Postmark (https://postmarkapp.com).
     *
     * Env vars: MAIL_API_KEY
     *
     * @internal Called by core_email_send(). Override to customise Postmark behaviour.
     */
    function core_email_send_postmark(
        string $to,
        string $subject,
        string $html_body,
        string $text_body,
        string $from_email,
        string $from_name
    ): bool {
        $from    = $from_name ? "{$from_name} <{$from_email}>" : $from_email;
        $api_key = getenv('MAIL_API_KEY') ?: '';

        $payload = json_encode([
            'From'     => $from,
            'To'       => $to,
            'Subject'  => $subject,
            'HtmlBody' => $html_body,
            'TextBody' => $text_body,
        ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);

        $result = _core_email_http_post(
            'https://api.postmarkapp.com/email',
            [
                'Content-Type: application/json',
                'Accept: application/json',
                'X-Postmark-Server-Token: ' . $api_key,
            ],
            $payload
        );

        return $result['code'] >= 200 && $result['code'] < 300;
    }
}

if (!function_exists('core_email_send_mail')) {
    /**
     * Send via PHP's mail() function (local dev / fallback).
     * Builds a multipart/alternative MIME message.
     * For local development, use Mailpit to catch outgoing mail.
     *
     * @internal Called by core_email_send(). Override to customise mail() behaviour.
     */
    function core_email_send_mail(
        string $to,
        string $subject,
        string $html_body,
        string $text_body,
        string $from_email,
        string $from_name
    ): bool {
        $from      = $from_name ? "{$from_name} <{$from_email}>" : $from_email;
        $boundary  = 'b' . bin2hex(random_bytes(8));

        $headers = implode("\r\n", [
            'MIME-Version: 1.0',
            'Content-Type: multipart/alternative; boundary="' . $boundary . '"',
            'From: ' . $from,
        ]);

        $body = implode("\r\n", [
            '--' . $boundary,
            'Content-Type: text/plain; charset=UTF-8',
            'Content-Transfer-Encoding: quoted-printable',
            '',
            quoted_printable_encode($text_body ?: strip_tags($html_body)),
            '--' . $boundary,
            'Content-Type: text/html; charset=UTF-8',
            'Content-Transfer-Encoding: quoted-printable',
            '',
            quoted_printable_encode($html_body),
            '--' . $boundary . '--',
        ]);

        return mail($to, $subject, $body, $headers);
    }
}
