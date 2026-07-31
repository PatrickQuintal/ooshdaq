ALTER TABLE users ADD COLUMN last_seen_at TEXT;

CREATE TABLE collection_entries_next (
    user_id TEXT NOT NULL REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    item_id TEXT NOT NULL
        REFERENCES catalog_items(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    owned INTEGER NOT NULL CHECK (owned BETWEEN 0 AND 1000000),
    available INTEGER NOT NULL CHECK (available BETWEEN 0 AND owned),
    available_since_at TEXT,
    wanted INTEGER NOT NULL CHECK (wanted BETWEEN 0 AND 1000000),
    wanted_since_at TEXT,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, item_id),
    CHECK (owned > 0 OR available > 0 OR wanted > 0),
    CHECK (available = 0 OR wanted = 0),
    CHECK (
        (available = 0 AND available_since_at IS NULL)
        OR
        (available > 0 AND available_since_at IS NOT NULL)
    ),
    CHECK (
        (wanted = 0 AND wanted_since_at IS NULL)
        OR
        (wanted > 0 AND wanted_since_at IS NOT NULL)
    )
) STRICT, WITHOUT ROWID;

INSERT INTO collection_entries_next(
    user_id,
    item_id,
    owned,
    available,
    available_since_at,
    wanted,
    wanted_since_at,
    updated_at
)
SELECT
    user_id,
    item_id,
    owned,
    available,
    CASE WHEN available > 0 THEN updated_at END,
    wanted,
    CASE WHEN wanted > 0 THEN updated_at END,
    updated_at
FROM collection_entries;

DROP TABLE collection_entries;
ALTER TABLE collection_entries_next RENAME TO collection_entries;

CREATE INDEX collection_entries_offers_idx
    ON collection_entries(item_id, user_id, available, available_since_at)
    WHERE available > 0;
CREATE INDEX collection_entries_wants_idx
    ON collection_entries(item_id, user_id, wanted, wanted_since_at)
    WHERE wanted > 0;
CREATE INDEX collection_entries_user_offers_idx
    ON collection_entries(user_id, item_id, available, available_since_at)
    WHERE available > 0;
CREATE INDEX collection_entries_user_wants_idx
    ON collection_entries(user_id, item_id, wanted, wanted_since_at)
    WHERE wanted > 0;
