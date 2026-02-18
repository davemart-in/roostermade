<?php

/**
 * core/php/db.php
 *
 * SQLite helpers for RoosterMade apps.
 * All functions are wrapped in if (!function_exists()) so app-level
 * overrides in overrides/{app}/php/db.php take precedence.
 */

if (!function_exists('db_connect')) {
    /**
     * Open a SQLite connection with WAL mode and foreign keys enabled.
     *
     * @param string $path Absolute or relative path to the .db file.
     * @return PDO
     */
    function db_connect(string $path): PDO
    {
        $db = new PDO('sqlite:' . $path);

        $db->setAttribute(PDO::ATTR_ERRMODE,            PDO::ERRMODE_EXCEPTION);
        $db->setAttribute(PDO::ATTR_DEFAULT_FETCH_MODE, PDO::FETCH_ASSOC);

        $db->exec('PRAGMA journal_mode = WAL');
        $db->exec('PRAGMA foreign_keys = ON');

        return $db;
    }
}

if (!function_exists('db_get')) {
    /**
     * Execute a SELECT and return the first row, or null if no rows match.
     *
     * @param PDO    $db
     * @param string $sql
     * @param array  $params Positional or named parameters.
     * @return array|null
     */
    function db_get(PDO $db, string $sql, array $params = []): array|null
    {
        $stmt = $db->prepare($sql);
        $stmt->execute($params);
        $row = $stmt->fetch();
        return $row !== false ? $row : null;
    }
}

if (!function_exists('db_all')) {
    /**
     * Execute a SELECT and return all matching rows.
     *
     * @param PDO    $db
     * @param string $sql
     * @param array  $params Positional or named parameters.
     * @return array
     */
    function db_all(PDO $db, string $sql, array $params = []): array
    {
        $stmt = $db->prepare($sql);
        $stmt->execute($params);
        return $stmt->fetchAll();
    }
}

if (!function_exists('db_run')) {
    /**
     * Execute a statement (INSERT, UPDATE, DELETE, PRAGMA, etc.).
     * Returns true on success; throws PDOException on failure.
     *
     * @param PDO    $db
     * @param string $sql
     * @param array  $params Positional or named parameters.
     * @return bool
     */
    function db_run(PDO $db, string $sql, array $params = []): bool
    {
        $stmt = $db->prepare($sql);
        return $stmt->execute($params);
    }
}

if (!function_exists('db_insert')) {
    /**
     * Execute an INSERT and return the last inserted row ID.
     *
     * @param PDO    $db
     * @param string $sql
     * @param array  $params Positional or named parameters.
     * @return int
     */
    function db_insert(PDO $db, string $sql, array $params = []): int
    {
        $stmt = $db->prepare($sql);
        $stmt->execute($params);
        return (int) $db->lastInsertId();
    }
}
