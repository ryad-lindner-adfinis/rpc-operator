package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"sort"
	"strings"
)

func main() {
	// Check for marker file (.drift-check-enabled) — Phase 1 skips, Phase 2+ enforces
	markerFile := ".drift-check-enabled"
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		fmt.Println("INFO: .drift-check-enabled not found — drift check disabled (Phase 1)")
		return
	}

	// Extract fields from CRD types
	pipelineFields := collectGoFieldsFromSource("api/v1alpha1/pipeline_types.go", "PipelineSpec")
	clusterFields := collectGoFieldsFromSource("api/v1alpha1/pipelinecluster_types.go", "PipelineClusterSpec")

	// Extract headings from documentation
	pipelineDocHeadings := collectDocHeadings("docs/user/reference/pipeline-crd.md")
	clusterDocHeadings := collectDocHeadings("docs/user/reference/pipelinecluster-crd.md")

	// Compare and report
	pipelineDiff := diff(pipelineFields, pipelineDocHeadings, "Pipeline")
	clusterDiff := diff(clusterFields, clusterDocHeadings, "PipelineCluster")

	if pipelineDiff != "" || clusterDiff != "" {
		fmt.Println(pipelineDiff)
		fmt.Println(clusterDiff)
		os.Exit(1)
	}

	fmt.Println("INFO: All CRD fields documented")
}

func collectGoFieldsFromSource(filepath string, typeName string) []string {
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, filepath, nil, 0)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", filepath, err)
	}

	var fields []string
	inTarget := false

	for _, decl := range tree.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			inTarget = true
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

	if !inTarget {
		return fields
	}

	sort.Strings(fields)
	return fields
}

func extractJSONTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}

	tag := field.Tag.Value
	if !strings.Contains(tag, `json:"`) {
		return ""
	}

	start := strings.Index(tag, `json:"`) + 6
	end := strings.Index(tag[start:], `"`)
	if end == -1 {
		return ""
	}

	jsonField := tag[start : start+end]
	if jsonField == "-" {
		return ""
	}

	return jsonField
}

func collectDocHeadings(filepath string) []string {
	content, err := os.ReadFile(filepath)
	if err != nil {
		log.Fatalf("Failed to read %s: %v", filepath, err)
	}

	lines := strings.Split(string(content), "\n")
	var headings []string

	// Find "## Spec" or "## Status" sections, then extract ### (field) headings
	inFieldSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## Spec") || strings.HasPrefix(line, "## Status") {
			inFieldSection = true
			continue
		}
		if strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "### ") {
			inFieldSection = false
			continue
		}
		if inFieldSection && strings.HasPrefix(line, "### ") {
			heading := strings.TrimPrefix(line, "### ")
			heading = strings.TrimSpace(heading)
			// Extract field name (text before the first space or paren)
			parts := strings.FieldsFunc(heading, func(r rune) bool { return r == ' ' || r == '(' })
			if len(parts) > 0 {
				headings = append(headings, parts[0])
			}
		}
	}

	sort.Strings(headings)
	return headings
}

func diff(goFields, docHeadings []string, crdName string) string {
	goFieldMap := make(map[string]bool)
	docHeadingMap := make(map[string]bool)

	for _, f := range goFields {
		goFieldMap[f] = true
	}
	for _, h := range docHeadings {
		docHeadingMap[h] = true
	}

	var missing, extra []string

	for _, f := range goFields {
		if !docHeadingMap[f] {
			missing = append(missing, f)
		}
	}

	for _, h := range docHeadings {
		if !goFieldMap[h] {
			extra = append(extra, h)
		}
	}

	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("ERROR: %s CRD reference drift detected:\n", crdName))

	if len(missing) > 0 {
		output.WriteString("  Missing in docs:\n")
		for _, f := range missing {
			output.WriteString(fmt.Sprintf("    - %s\n", f))
		}
	}

	if len(extra) > 0 {
		output.WriteString("  Extra in docs (not in code):\n")
		for _, h := range extra {
			output.WriteString(fmt.Sprintf("    - %s\n", h))
		}
	}

	return output.String()
}
