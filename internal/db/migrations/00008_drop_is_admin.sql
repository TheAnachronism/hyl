-- +goose Up
-- users.is_admin was set for the first registration and returned to the client,
-- but nothing ever consulted it: no handler, no middleware, no template. Either
-- an admin gate existed or the flag did not, and it did not.
ALTER TABLE users DROP COLUMN is_admin;

-- +goose Down
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT 0;
