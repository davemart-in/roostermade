<?php

/**
 * core/php/router.php
 *
 * HTTP router for RoosterMade apps.
 * Supports {param} placeholders in route patterns.
 * All functions are wrapped in if (!function_exists()) so app-level
 * overrides in overrides/{app}/php/router.php take precedence.
 */

if (!function_exists('_core_router_routes')) {
    /**
     * Internal: return a reference to the static route registry.
     *
     * @return array
     */
    function &_core_router_routes(): array
    {
        static $routes = [];
        return $routes;
    }
}

if (!function_exists('core_router_route')) {
    /**
     * Register a route.
     *
     * Patterns may include {param} placeholders, e.g. /api/customers/{id}.
     * Each placeholder matches one or more non-slash characters.
     *
     * @param string   $method  HTTP method ('GET', 'POST', 'DELETE', etc.)
     * @param string   $pattern URI pattern, e.g. '/api/customers/{id}'
     * @param callable $handler Called with (array $params) on match.
     * @return void
     */
    function core_router_route(string $method, string $pattern, callable $handler): void
    {
        $method = strtoupper($method);

        // Convert {param} placeholders to named regex captures
        $regex = preg_replace('#\{([a-zA-Z_][a-zA-Z0-9_]*)\}#', '(?P<$1>[^/]+)', $pattern);
        $regex = '#^' . $regex . '$#';

        $routes   = &_core_router_routes();
        $routes[] = [
            'method'  => $method,
            'regex'   => $regex,
            'handler' => $handler,
        ];
    }
}

if (!function_exists('core_router_dispatch')) {
    /**
     * Match the current request against registered routes and call the handler.
     *
     * Named {param} captures from the pattern are passed to the handler as an
     * associative array. Sends a 404 JSON response if no route matches.
     *
     * @return void
     */
    function core_router_dispatch(): void
    {
        $method = strtoupper($_SERVER['REQUEST_METHOD']);
        $uri    = strtok($_SERVER['REQUEST_URI'], '?');

        foreach (_core_router_routes() as $route) {
            if ($route['method'] !== $method) {
                continue;
            }

            if (!preg_match($route['regex'], $uri, $matches)) {
                continue;
            }

            // Keep only named captures (string keys), discard numeric keys
            $params = array_filter(
                $matches,
                fn($key) => is_string($key),
                ARRAY_FILTER_USE_KEY
            );

            ($route['handler'])($params);
            return;
        }

        core_response_json(['error' => 'Not found'], 404);
    }
}
