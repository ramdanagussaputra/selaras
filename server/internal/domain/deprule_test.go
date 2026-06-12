package domain_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

const domainPrefix = "github.com/ramdanaguss/selaras/server/internal/domain"

// TestDomainImportsStdlibOnly enforces the Clean Architecture dependency rule
// (design D5): every package under internal/domain may depend only on the Go
// standard library and other domain packages. It runs as a plain test so
// `make test` and the CI server job enforce the rule with no extra wiring.
func TestDomainImportsStdlibOnly(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-json", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps failed: %v", err)
	}

	type pkg struct {
		ImportPath string
		Standard   bool
	}

	var violations []string
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}

		if p.Standard || strings.HasPrefix(p.ImportPath, domainPrefix) {
			continue
		}
		violations = append(violations, p.ImportPath)
	}

	if len(violations) > 0 {
		t.Errorf(
			"internal/domain must import stdlib only (dependency rule); offending imports:\n  %s",
			strings.Join(violations, "\n  "),
		)
	}
}
