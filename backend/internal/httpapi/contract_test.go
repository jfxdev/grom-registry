package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jfxdev/grom/backend/api"
)

func TestOpenAPIContractIsValid(t *testing.T) {
	if err := validateOpenAPI(api.OpenAPI); err != nil {
		t.Fatalf("OpenAPI contract is invalid: %v", err)
	}
}

func TestOpenAPIContractRejectsInvalidDocuments(t *testing.T) {
	invalid := []byte("openapi: 3.0.3\ninfo: {}\npaths: {}\n")
	if err := validateOpenAPI(invalid); err == nil {
		t.Fatal("expected invalid OpenAPI document to be rejected")
	}
}

func TestImplementedRoutesAreDocumented(t *testing.T) {
	document, err := loadOpenAPI(api.OpenAPI)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}

	implemented := routeOperations(t, (&Server{}).routes())
	contract := contractOperations(document)

	var undocumented []string
	for operation := range implemented {
		if !contract[operation] {
			undocumented = append(undocumented, operation)
		}
	}
	sort.Strings(undocumented)
	if len(undocumented) > 0 {
		t.Fatalf("implemented routes are missing from OpenAPI: %s", strings.Join(undocumented, ", "))
	}
}

func TestDocumentedOperationsAreRegistered(t *testing.T) {
	document, err := loadOpenAPI(api.OpenAPI)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}

	implemented := routeOperations(t, (&Server{}).routes())
	contract := contractOperations(document)

	var unregistered []string
	for operation := range contract {
		if !implemented[operation] {
			unregistered = append(unregistered, operation)
		}
	}
	sort.Strings(unregistered)
	if len(unregistered) > 0 {
		t.Fatalf("OpenAPI operations are not registered: %s", strings.Join(unregistered, ", "))
	}
}

func validateOpenAPI(data []byte) error {
	document, err := loadOpenAPI(data)
	if err != nil {
		return err
	}
	return document.Validate(context.Background())
}

func loadOpenAPI(data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	return loader.LoadFromData(data)
}

func routeOperations(t *testing.T, router chi.Router) map[string]bool {
	t.Helper()
	operations := make(map[string]bool)
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if route == "/v2/" || strings.HasPrefix(route, "/v2/") ||
			route == "/api/docs" || route == "/api/openapi.yaml" {
			return nil
		}
		operations[strings.ToUpper(method)+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk registered routes: %v", err)
	}
	return operations
}

func contractOperations(document *openapi3.T) map[string]bool {
	operations := make(map[string]bool)
	for route, item := range document.Paths.Map() {
		for method, operation := range map[string]*openapi3.Operation{
			"GET": item.Get, "PUT": item.Put, "POST": item.Post,
			"DELETE": item.Delete, "PATCH": item.Patch, "HEAD": item.Head,
			"OPTIONS": item.Options, "TRACE": item.Trace,
		} {
			if operation != nil {
				operations[method+" "+route] = true
			}
		}
	}
	return operations
}
