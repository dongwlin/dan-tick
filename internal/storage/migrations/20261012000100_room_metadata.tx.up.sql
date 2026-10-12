ALTER TABLE rooms ADD COLUMN cover_url TEXT NOT NULL DEFAULT '';

--bun:split

ALTER TABLE rooms ADD COLUMN metadata_checked_at INTEGER NOT NULL DEFAULT 0;
