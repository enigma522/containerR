CREATE TABLE IF NOT EXISTS CONTAINERS (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    image_ref TEXT NOT NULL,
    image_digest TEXT NOT NULL,
    rootfs TEXT NOT NULL,
    cmd TEXT NOT NULL,
    entrypoint TEXT NOT NULL,
    env TEXT NOT NULL,
    working_dir TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    user TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL
);
