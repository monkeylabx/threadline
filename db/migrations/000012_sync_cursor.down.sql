BEGIN;

DROP TABLE domain.sync_cursors;
DROP FUNCTION domain.enforce_sync_cursor_lifecycle();

COMMIT;
