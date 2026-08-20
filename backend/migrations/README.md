# Migrations

Reserved for SQL schema migrations. M01 deliberately creates no tables — the
first migration (the `users` table) lands with M02. The specific migration
tool is chosen at that point rather than pre-decided here, to avoid adding a
dependency before there's a schema for it to manage.
