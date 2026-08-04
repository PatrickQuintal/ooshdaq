CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL CHECK (length(checksum) = 64),
    applied_at TEXT NOT NULL
) STRICT;

CREATE TABLE brands (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL CHECK (name <> ''),
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'retired'))
) STRICT;

CREATE TABLE catalogs (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    brand_id TEXT NOT NULL REFERENCES brands(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (name <> ''),
    edition TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'retired')),
    sort_order INTEGER NOT NULL CHECK (sort_order > 0)
) STRICT;

CREATE INDEX catalogs_navigation_idx
    ON catalogs(status, sort_order, id);
CREATE INDEX catalogs_brand_navigation_idx
    ON catalogs(brand_id, status, sort_order, id);

CREATE TABLE catalog_sources (
    catalog_id TEXT PRIMARY KEY
        REFERENCES catalogs(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    source_page_url TEXT NOT NULL DEFAULT '',
    retrieved_at TEXT NOT NULL DEFAULT '',
    reviewed_at TEXT NOT NULL,
    review_reference TEXT NOT NULL,
    attribution TEXT NOT NULL DEFAULT '',
    rights_status TEXT NOT NULL CHECK (
        rights_status IN (
            'original',
            'licensed',
            'permissioned-link',
            'source-hosted-link',
            'unknown',
            'restricted'
        )
    ),
    license TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE catalog_items (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    catalog_id TEXT NOT NULL
        REFERENCES catalogs(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    number INTEGER NOT NULL CHECK (number > 0),
    name TEXT NOT NULL CHECK (name <> ''),
    universe TEXT NOT NULL DEFAULT '',
    variant TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'retired')),
    retirement_effective_date TEXT NOT NULL DEFAULT '',
    retirement_reason TEXT NOT NULL DEFAULT '',
    UNIQUE (catalog_id, number),
    CHECK (
        (status = 'retired' AND retirement_effective_date <> '' AND retirement_reason <> '')
        OR
        (status <> 'retired' AND retirement_effective_date = '' AND retirement_reason = '')
    )
) STRICT;

CREATE INDEX catalog_items_page_idx
    ON catalog_items(catalog_id, status, number, id);
CREATE INDEX catalog_items_universe_page_idx
    ON catalog_items(catalog_id, universe, status, number, id);

CREATE TABLE item_assets (
    item_id TEXT PRIMARY KEY
        REFERENCES catalog_items(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    asset_url TEXT NOT NULL DEFAULT '',
    placeholder INTEGER NOT NULL CHECK (placeholder IN (0, 1)),
    rights_status TEXT NOT NULL CHECK (
        rights_status IN (
            'original',
            'licensed',
            'permissioned-link',
            'source-hosted-link',
            'unknown',
            'restricted'
        )
    ),
    license TEXT NOT NULL DEFAULT '',
    attribution TEXT NOT NULL DEFAULT '',
    permission_reference TEXT NOT NULL DEFAULT '',
    source_page_url TEXT NOT NULL DEFAULT '',
    retrieved_at TEXT NOT NULL DEFAULT '',
    reviewed_at TEXT NOT NULL,
    review_reference TEXT NOT NULL,
    transformation TEXT NOT NULL,
    sha256 TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE catalog_imports (
    catalog_id TEXT PRIMARY KEY
        REFERENCES catalogs(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    manifest_version INTEGER NOT NULL CHECK (manifest_version > 0),
    manifest_digest TEXT NOT NULL CHECK (length(manifest_digest) = 64),
    imported_at TEXT NOT NULL
) STRICT;

CREATE TABLE users (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    display_name TEXT NOT NULL CHECK (display_name <> ''),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_seen_at TEXT
) STRICT;

CREATE TABLE collection_entries (
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

CREATE TRIGGER brands_keep_identity
BEFORE UPDATE OF id ON brands
BEGIN
    SELECT RAISE(ABORT, 'brand identity is immutable');
END;

CREATE TRIGGER brands_keep_history
BEFORE DELETE ON brands
BEGIN
    SELECT RAISE(ABORT, 'brand history must be retained');
END;

CREATE TRIGGER brands_keep_retirement
BEFORE UPDATE OF status ON brands
WHEN OLD.status = 'retired' AND NEW.status <> 'retired'
BEGIN
    SELECT RAISE(ABORT, 'retired brand cannot be reactivated');
END;

CREATE TRIGGER catalogs_keep_identity
BEFORE UPDATE OF id, brand_id, sort_order ON catalogs
BEGIN
    SELECT RAISE(ABORT, 'catalog identity and order are immutable');
END;

CREATE TRIGGER catalogs_keep_history
BEFORE DELETE ON catalogs
BEGIN
    SELECT RAISE(ABORT, 'catalog history must be retained');
END;

CREATE TRIGGER catalogs_keep_retirement
BEFORE UPDATE OF status ON catalogs
WHEN OLD.status = 'retired' AND NEW.status <> 'retired'
BEGIN
    SELECT RAISE(ABORT, 'retired catalog cannot be reactivated');
END;

CREATE TRIGGER catalog_items_keep_identity
BEFORE UPDATE OF id, catalog_id, number ON catalog_items
BEGIN
    SELECT RAISE(ABORT, 'catalog item identity and number are immutable');
END;

CREATE TRIGGER catalog_items_keep_history
BEFORE DELETE ON catalog_items
BEGIN
    SELECT RAISE(ABORT, 'catalog item history must be retained');
END;

CREATE TRIGGER catalog_items_keep_retirement
BEFORE UPDATE OF status ON catalog_items
WHEN OLD.status = 'retired' AND NEW.status <> 'retired'
BEGIN
    SELECT RAISE(ABORT, 'retired catalog item cannot be reactivated');
END;
