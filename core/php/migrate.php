<?php

/**
 * core/php/migrate.php
 *
 * Schema migration runner for RoosterMade apps.
 * All functions are wrapped in if (!function_exists()) so app-level
 * overrides in overrides/{app}/php/migrate.php take precedence.
 */

if (!function_exists('core_migrate_run')) {
    /**
     * Apply any unapplied SQL migrations from a directory.
     *
     * Migrations are .sql files sorted lexicographically by filename.
     * Numeric prefixes (001_, 002_, ...) ensure correct ordering.
     * Already-applied migrations are skipped. Running twice is safe.
     *
     * Each migration is applied in its own transaction. A failure on
     * migration N leaves all prior migrations committed.
     *
     * @param PDO    $db             Open database connection.
     * @param string $migrations_dir Path to directory containing .sql files.
     * @return void
     * @throws PDOException If a migration fails.
     */
    function core_migrate_run(PDO $db, string $migrations_dir): void
    {
        // Ensure the version tracking table exists
        $db->exec('
            CREATE TABLE IF NOT EXISTS schema_versions (
                version    TEXT PRIMARY KEY,
                applied_at TEXT NOT NULL DEFAULT (datetime(\'now\'))
            )
        ');

        // Load already-applied versions for O(1) lookup
        $applied = $db->query('SELECT version FROM schema_versions')
                      ->fetchAll(PDO::FETCH_COLUMN);
        $applied = array_flip($applied);

        // Discover and sort migration files
        $files = glob($migrations_dir . '/*.sql');
        if (!$files) {
            return;
        }
        sort($files);

        foreach ($files as $file) {
            $version = basename($file);

            if (isset($applied[$version])) {
                continue;
            }

            $sql = file_get_contents($file);

            $db->beginTransaction();
            try {
                $db->exec($sql);
                $db->prepare('INSERT INTO schema_versions (version) VALUES (?)')->execute([$version]);
                $db->commit();
            } catch (Throwable $e) {
                $db->rollBack();
                throw $e;
            }
        }
    }
}
