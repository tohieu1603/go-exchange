-- External schema for sqlc analysis ONLY — never run as a migration.
-- The admin deposit/withdrawal list queries filter by the owning user's email,
-- which lives in the `users` table owned by auth-service in the shared database.
-- Declaring a compatible subset here lets sqlc type-check those JOINs without
-- wallet-service claiming ownership of (or migrating) the users table.
CREATE TABLE IF NOT EXISTS users (
    id    bigint PRIMARY KEY,
    email text   NOT NULL
);
