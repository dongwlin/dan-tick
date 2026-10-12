ALTER TABLE rooms DROP COLUMN metadata_checked_at;

--bun:split

ALTER TABLE rooms DROP COLUMN cover_url;
