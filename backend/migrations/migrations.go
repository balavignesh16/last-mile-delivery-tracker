// Package migrations embeds the project's SQL migration files so they can
// be applied reproducibly regardless of the process's working directory —
// go:embed bakes the files into the compiled binary at build time, which
// sidesteps the entire class of cwd-relative-path bug that affected M01's
// .env loading (see internal/config/dotenv.go's LoadFirstDotEnv comment).
// It also means the Docker image needs no separate step to carry the SQL
// files at runtime.
//
// See internal/database/migrate.go for the runner that applies these.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
