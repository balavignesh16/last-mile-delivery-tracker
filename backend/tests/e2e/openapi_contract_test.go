//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// openAPISpec is deliberately minimal — only the one section this
// contract test actually checks (paths -> methods). It ignores every
// other OpenAPI field (schemas, parameters, responses, ...) rather than
// modeling the whole spec, since the goal here is structural drift
// detection (does every real route have a documented path and vice
// versa), not full schema validation.
type openAPISpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

func loadOpenAPISpec(t *testing.T) openAPISpec {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs/openapi.yaml: %v", err)
	}
	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse docs/openapi.yaml: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("docs/openapi.yaml parsed with zero paths — something is structurally wrong")
	}
	return spec
}

// realRoutes walks the actual, fully-mounted router (every module,
// exactly as setupE2E/main.go build it) via chi's own route-introspection
// API and returns "METHOD /path" for every real, registered business
// route. chi's {param} path-parameter syntax already matches
// docs/openapi.yaml's own — both were written by hand to name path
// parameters identically (id, zoneID, areaID, rateCardID, slabID), so
// no translation step is needed between the two.
func realRoutes(t *testing.T, router chi.Router) map[string]bool {
	t.Helper()
	routes := map[string]bool{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk() error: %v", err)
	}
	return routes
}

// TestOpenAPIContract_DocumentedPathsMatchRealRoutes is the automated
// drift check the M12 audit's manual path-by-path comparison was
// missing: every method+path docs/openapi.yaml declares must exist as
// a real, mounted route, and every real business route (under
// /api/v1/* or the unversioned /health) must be documented — in either
// direction, a mismatch means the document and the router have drifted
// apart.
func TestOpenAPIContract_DocumentedPathsMatchRealRoutes(t *testing.T) {
	router, _, _ := setupE2E(t)
	spec := loadOpenAPISpec(t)

	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	real := realRoutes(t, router)

	var missingFromRouter, missingFromDocs []string
	for key := range documented {
		if !real[key] {
			missingFromRouter = append(missingFromRouter, key)
		}
	}
	for key := range real {
		// OPTIONS entries are chi's own auto-registered CORS/preflight
		// stubs on mounted sub-routers, not a real, independently
		// documented endpoint — every real business method is already
		// covered by its GET/POST/PUT/DELETE entry.
		if strings.HasPrefix(key, "OPTIONS ") {
			continue
		}
		if !documented[key] {
			missingFromDocs = append(missingFromDocs, key)
		}
	}

	sort.Strings(missingFromRouter)
	sort.Strings(missingFromDocs)

	if len(missingFromRouter) > 0 {
		t.Errorf("docs/openapi.yaml documents %d path(s) that don't exist as real routes:\n%s", len(missingFromRouter), strings.Join(missingFromRouter, "\n"))
	}
	if len(missingFromDocs) > 0 {
		t.Errorf("%d real route(s) are not documented in docs/openapi.yaml:\n%s", len(missingFromDocs), strings.Join(missingFromDocs, "\n"))
	}
}
