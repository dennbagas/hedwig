CREATE TABLE cicd_retries_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id       INTEGER NOT NULL,
    message_id    INTEGER NOT NULL,
    run_id        INTEGER NOT NULL,
    repo          TEXT    NOT NULL,
    status        TEXT    NOT NULL DEFAULT 'pending',
    workflow_name TEXT    NOT NULL DEFAULT '',
    message_text  TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cicd_retries_old (id, chat_id, message_id, run_id, repo, status, workflow_name, message_text, created_at)
SELECT r.id,
       CAST(COALESCE(t.chat_ref, '0') AS INTEGER),
       CAST(COALESCE(t.message_ref, '0') AS INTEGER),
       r.run_id, r.repo, r.status, r.workflow_name, COALESCE(t.message_text, ''), r.created_at
FROM cicd_retries r
LEFT JOIN cicd_retry_targets t ON t.retry_id = r.id AND t.platform = 'telegram';

DROP TABLE cicd_retry_targets;

DROP TABLE cicd_retries;

ALTER TABLE cicd_retries_old RENAME TO cicd_retries;
