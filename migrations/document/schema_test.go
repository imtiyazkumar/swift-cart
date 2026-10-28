package document

import (
	"regexp"
	"strings"
	"testing"
)

// A fresh installation should not replay superseded versions of its functions.
func TestCanonicalBaselineDefinesEachFunctionOnce(t *testing.T) {
	declarations := regexp.MustCompile(`(?i)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+([a-z_][a-z_0-9]*)`).FindAllStringSubmatch(SQL, -1)
	if len(declarations) == 0 {
		t.Fatal("baseline contains no behavior functions")
	}
	seen := map[string]bool{}
	for _, declaration := range declarations {
		name := strings.ToLower(declaration[1])
		if seen[name] {
			t.Fatalf("baseline redefines %s; keep only its final implementation", name)
		}
		seen[name] = true
	}
}
