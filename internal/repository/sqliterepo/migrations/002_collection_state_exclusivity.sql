CREATE TABLE collection_entries_next (
    user_id TEXT NOT NULL REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    item_id TEXT NOT NULL
        REFERENCES catalog_items(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    owned INTEGER NOT NULL CHECK (owned BETWEEN 0 AND 1000000),
    available INTEGER NOT NULL CHECK (available BETWEEN 0 AND owned),
    wanted INTEGER NOT NULL CHECK (wanted BETWEEN 0 AND 1000000),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, item_id),
    CHECK (owned > 0 OR available > 0 OR wanted > 0),
    CHECK (wanted = 0 OR (owned = 0 AND available = 0))
) STRICT, WITHOUT ROWID;

INSERT INTO collection_entries_next(
    user_id,
    item_id,
    owned,
    available,
    wanted,
    updated_at
)
SELECT
    user_id,
    item_id,
    CASE WHEN wanted > 0 THEN 0 ELSE owned END,
    CASE WHEN wanted > 0 THEN 0 ELSE available END,
    wanted,
    updated_at
FROM collection_entries;

DROP TABLE collection_entries;
ALTER TABLE collection_entries_next RENAME TO collection_entries;

CREATE INDEX collection_entries_user_item_idx
    ON collection_entries(user_id, item_id);
CREATE INDEX collection_entries_offers_idx
    ON collection_entries(item_id, user_id, available)
    WHERE available > 0;
CREATE INDEX collection_entries_wants_idx
    ON collection_entries(item_id, user_id, wanted)
    WHERE wanted > 0;
CREATE INDEX collection_entries_user_offers_idx
    ON collection_entries(user_id, item_id, available)
    WHERE available > 0;
CREATE INDEX collection_entries_user_wants_idx
    ON collection_entries(user_id, item_id, wanted)
    WHERE wanted > 0;
