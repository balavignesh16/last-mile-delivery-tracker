# Unit tests

Unit tests live next to the code they test (`internal/config/config_test.go`,
`internal/server/health_test.go`, etc.), following standard Go convention —
that's what makes `go test ./...` discover them automatically and keeps a
test physically next to the function it exercises.

This directory is kept as part of the repository's structural layout for
any cross-package unit-level suite that doesn't belong to a single
`internal/` package. None exists yet as of M01.
