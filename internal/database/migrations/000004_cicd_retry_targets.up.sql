CREATE TABLE cicd_retry_targets (
    retry_id     INTEGER NOT NULL REFERENCES cicd_retries(id),
    platform     TEXT NOT NULL,
    chat_ref     TEXT NOT NULL,
    message_ref  TEXT NOT NULL,
    message_text TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (retry_id, platform)
);

INSERT INTO cicd_retry_targets (retry_id, platform, chat_ref, message_ref, message_text)
SELECT id, 'telegram', CAST(chat_id AS TEXT), CAST(message_id AS TEXT), message_text
FROM cicd_retries;

CREATE TABLE cicd_retries_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER NOT NULL,
    repo          TEXT    NOT NULL,
    status        TEXT    NOT NULL DEFAULT 'pending',
    workflow_name TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cicd_retries_new (id, run_id, repo, status, workflow_name, created_at)
SELECT id, run_id, repo, status, workflow_name, created_at
FROM cicd_retries;

DROP TABLE cicd_retries;

ALTER TABLE cicd_retries_new RENAME TO cicd_retries;
