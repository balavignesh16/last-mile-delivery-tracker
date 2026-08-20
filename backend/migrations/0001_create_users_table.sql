-- M02: the users table backs all three roles (customer, delivery agent,
-- admin) in one table, per the frozen architecture's decision to fold the
-- earlier "customers" table into "users" (no customer-only fields exist).
--
-- gen_random_uuid() is built into PostgreSQL core since v13 — no pgcrypto
-- or other extension needed.

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    phone         TEXT,
    role          TEXT NOT NULL CHECK (role IN ('CUSTOMER', 'DELIVERY_AGENT', 'ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
