package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

func TestCollectGoFieldsExtractsJSONTags(t *testing.T) {
	code := `
package main
type PipelineSpec struct {
	RawYAML string ` + "`json:\"rawYAML,omitempty\"`" + `
	Enabled bool   ` + "`json:\"enabled,omitempty\"`" + `
	Hidden  string ` + "`json:\"-\"`" + `
}
`

	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "test.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	var fields []string
	for _, decl := range tree.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != "PipelineSpec" {
				continue
			}
			structType := typeSpec.Type.(*ast.StructType)
			for _, field := range structType.Fields.List {
				for range field.Names {
					jsonTag := extractJSONTag(field)
					if jsonTag != "" {
						name := strings.SplitN(jsonTag, ",", 2)[0]
						if name != "" && name != "-" {
							fields = append(fields, name)
						}
					}
				}
			}
		}
	}

	if len(fields) != 2 {
		t.Errorf("Expected 2 fields (rawYAML, enabled), got %d: %v", len(fields), fields)
	}
}

func TestCollectDocHeadingsParsesH3s(t *testing.T) {
	// Test markdown with H3 headings
	markdown := `
## Spec

### rawYAML

Configuration as YAML.

### enabled

Enable the pipeline.

## Status

### phase

Lifecycle phase.
`

	headings := extractHeadingsFromMarkdown(markdown)
	expected := []string{"enabled", "phase", "rawYAML"}

	if len(headings) != len(expected) {
		t.Errorf("Expected %d headings, got %d: %v", len(expected), len(headings), headings)
		return
	}

	for i, h := range headings {
		if h != expected[i] {
			t.Errorf("Heading %d: expected %q, got %q", i, expected[i], h)
		}
	}
}

func TestDiffReportsMissingDocHeading(t *testing.T) {
	goFields := []string{"field1", "field2", "field3"}
	docHeadings := []string{"field1", "field3"}

	result := diff(goFields, docHeadings, "Test")

	if !containsString(result, "field2") {
		t.Errorf("Expected 'field2' in diff output, got: %s", result)
	}
}

func TestDiffReportsExtraDocHeading(t *testing.T) {
	goFields := []string{"field1"}
	docHeadings := []string{"field1", "field2"}

	result := diff(goFields, docHeadings, "Test")

	if !containsString(result, "field2") {
		t.Errorf("Expected 'field2' in extra headings, got: %s", result)
	}
}

// Helper functions
func extractHeadingsFromMarkdown(markdown string) []string {
	var headings []string
	inFieldSection := false

	for _, line := range strings.Split(markdown, "\n") {
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "###") {
			inFieldSection = line == "## Spec" || line == "## Status"
		}
		if inFieldSection && strings.HasPrefix(line, "### ") {
			heading := strings.TrimPrefix(line, "### ")
			headings = append(headings, heading)
		}
	}

	sort.Strings(headings)
	return headings
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
