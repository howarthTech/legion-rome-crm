-- Photo gallery: albums of photos, authored in the CRM. Files live on the
-- CRM's data volume (/data/media); rows here hold the metadata. The public
-- website builds its gallery from GET /api/gallery.json, same as events/roster.
CREATE TABLE gallery_albums (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    slug        TEXT    NOT NULL UNIQUE,      -- URL slug: /gallery/<slug>/
    title       TEXT    NOT NULL,
    album_date  TEXT    NOT NULL DEFAULT '',  -- YYYY-MM-DD, for ordering/display
    description TEXT    NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE gallery_photos (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    album_id     INTEGER NOT NULL REFERENCES gallery_albums(id) ON DELETE CASCADE,
    filename     TEXT    NOT NULL,            -- stored file under /data/media
    caption      TEXT    NOT NULL DEFAULT '',
    content_type TEXT    NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_gallery_photos_album ON gallery_photos(album_id, sort_order, id);
