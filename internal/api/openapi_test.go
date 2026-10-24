// This test lives in package api (not api_test) so it can read the unexported
// endpoint table that Routes builds the mux from.
package api

import (
	"net/http"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/backend-pc4/restgo/internal/docs"
)

// specDocument is the subset of OpenAPI this test cares about.
type specDocument struct {
	Paths map[string]map[string]struct {
		OperationID string           `yaml:"operationId"`
		Security    []map[string]any `yaml:"security"`
	} `yaml:"paths"`
}

// httpMethods are the path-item keys that describe an operation. Anything else
// in a path item (summary, parameters, ...) is not a route.
var httpMethods = map[string]string{
	"get":     http.MethodGet,
	"put":     http.MethodPut,
	"post":    http.MethodPost,
	"delete":  http.MethodDelete,
	"options": http.MethodOptions,
	"head":    http.MethodHead,
	"patch":   http.MethodPatch,
	"trace":   http.MethodTrace,
}

func loadSpec(t *testing.T) specDocument {
	t.Helper()
	var doc specDocument
	if err := yaml.Unmarshal(docs.Spec(), &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("openapi.yaml declares no paths")
	}
	return doc
}

// TestOpenAPIMatchesRoutes is the guard against the hand-written spec drifting
// from the code. It compares both directions: an undocumented route is as much
// a bug as a documented route that does not exist.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	doc := loadSpec(t)

	// requireAuth by "METHOD /path", for the documented half of the API.
	inCode := make(map[string]bool)
	for _, ep := range (&Server{enableDocs: true}).endpoints() {
		if ep.inSpec {
			inCode[ep.method+" "+ep.path] = ep.requireAuth
		}
	}

	inSpec := make(map[string]bool)
	for path, item := range doc.Paths {
		for rawMethod, op := range item {
			method, ok := httpMethods[rawMethod]
			if !ok {
				continue
			}
			if op.OperationID == "" {
				t.Errorf("%s %s: missing operationId", method, path)
			}
			inSpec[method+" "+path] = len(op.Security) > 0
		}
	}

	for _, route := range sortedKeys(inCode) {
		if _, ok := inSpec[route]; !ok {
			t.Errorf("route %s is registered but missing from openapi.yaml", route)
		}
	}
	for _, route := range sortedKeys(inSpec) {
		if _, ok := inCode[route]; !ok {
			t.Errorf("openapi.yaml documents %s, which is not a registered route", route)
		}
	}

	// A route documented as public but actually protected is merely confusing;
	// the reverse is a security hole. Check both.
	for _, route := range sortedKeys(inCode) {
		specNeedsAuth, ok := inSpec[route]
		if !ok {
			continue
		}
		if codeNeedsAuth := inCode[route]; codeNeedsAuth != specNeedsAuth {
			t.Errorf("%s: requires auth = %t in code, but %t in openapi.yaml",
				route, codeNeedsAuth, specNeedsAuth)
		}
	}
}

// TestOpenAPIDeclaresBearerScheme pins the security scheme the protected
// endpoints reference, so `Authorize` in Swagger UI keeps working.
func TestOpenAPIDeclaresBearerScheme(t *testing.T) {
	var doc struct {
		Components struct {
			SecuritySchemes map[string]struct {
				Type         string `yaml:"type"`
				Scheme       string `yaml:"scheme"`
				BearerFormat string `yaml:"bearerFormat"`
			} `yaml:"securitySchemes"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(docs.Spec(), &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	scheme, ok := doc.Components.SecuritySchemes["bearerAuth"]
	if !ok {
		t.Fatal("openapi.yaml does not define the bearerAuth security scheme")
	}
	if scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Errorf("got type=%q scheme=%q, want http/bearer", scheme.Type, scheme.Scheme)
	}
	if scheme.BearerFormat != "JWT" {
		t.Errorf("got bearerFormat %q, want JWT", scheme.BearerFormat)
	}
}

// TestProtectedEndpointsReferenceBearerAuth makes sure each protected route
// names the scheme defined above rather than some undefined one.
func TestProtectedEndpointsReferenceBearerAuth(t *testing.T) {
	doc := loadSpec(t)

	for path, item := range doc.Paths {
		for rawMethod, op := range item {
			if _, ok := httpMethods[rawMethod]; !ok {
				continue
			}
			for _, requirement := range op.Security {
				for name := range requirement {
					if name != "bearerAuth" {
						t.Errorf("%s %s: references unknown security scheme %q",
							rawMethod, path, name)
					}
				}
			}
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
