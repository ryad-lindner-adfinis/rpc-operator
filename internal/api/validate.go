package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"k8s.io/apimachinery/pkg/runtime"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
	"github.com/insidegreen/rpc-operator-claude/internal/render"
)

var envVarNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidationError describes a single schema or render validation failure.
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidatePipeline schema-validates each component against the catalog and then
// performs a render dry-run. Returns nil if the pipeline is valid.
func ValidatePipeline(p *rpcv1alpha1.Pipeline, cat *catalog.Catalog) []ValidationError {
	if p.Spec.RawYAML != "" {
		// Raw mode: skip catalog validation; only check YAML syntax via render dry-run.
		var errs []ValidationError
		if _, err := render.RenderPipelineYAML(&p.Spec); err != nil {
			errs = append(errs, ValidationError{Path: "spec.rawYAML", Message: err.Error()})
		}
		errs = append(errs, validateSecretRefs(p.Spec.SecretRefs)...)
		if len(errs) == 0 {
			return nil
		}
		return errs
	}

	var errs []ValidationError
	errs = append(errs, validateComponent("spec.input", &p.Spec.Input, "inputs", cat)...)
	for i := range p.Spec.Processors {
		path := fmt.Sprintf("spec.processors[%d]", i)
		errs = append(errs, validateComponent(path, &p.Spec.Processors[i], "processors", cat)...)
		if p.Spec.Processors[i].Label == "" {
			errs = append(errs, ValidationError{
				Path:    fmt.Sprintf("spec.processors[%d].label", i),
				Message: "label is required for processors",
			})
		}
	}
	errs = append(errs, validateComponent("spec.output", &p.Spec.Output, "outputs", cat)...)

	errs = append(errs, validateSecretRefs(p.Spec.SecretRefs)...)

	if _, rerr := render.RenderPipelineYAML(&p.Spec); rerr != nil {
		errs = append(errs, ValidationError{Path: "spec", Message: "render failed: " + rerr.Error()})
	}
	return errs
}

// validateSecretRefs checks that every SecretRef has valid, non-duplicate fields.
func validateSecretRefs(refs []rpcv1alpha1.SecretRef) []ValidationError {
	var errs []ValidationError
	seen := map[string]bool{}
	for i, r := range refs {
		path := fmt.Sprintf("spec.secretRefs[%d]", i)
		if r.EnvVar == "" {
			errs = append(errs, ValidationError{Path: path + ".envVar", Message: "envVar is required"})
		} else if !envVarNameRe.MatchString(r.EnvVar) {
			errs = append(errs, ValidationError{Path: path + ".envVar", Message: "envVar must match [A-Za-z_][A-Za-z0-9_]*"})
		} else if seen[r.EnvVar] {
			errs = append(errs, ValidationError{Path: path + ".envVar", Message: fmt.Sprintf("duplicate envVar %q", r.EnvVar)})
		}
		seen[r.EnvVar] = true
		if r.SecretName == "" {
			errs = append(errs, ValidationError{Path: path + ".secretName", Message: "secretName is required"})
		}
		if r.Key == "" {
			errs = append(errs, ValidationError{Path: path + ".key", Message: "key is required"})
		}
	}
	return errs
}

func validateComponent(
	path string,
	c *rpcv1alpha1.ComponentSpec,
	category string,
	cat *catalog.Catalog,
) []ValidationError {
	if c.Type == "" {
		return []ValidationError{{Path: path + ".type", Message: "type is required"}}
	}
	comp, ok := cat.Get(category, c.Type)
	if !ok {
		return []ValidationError{{
			Path: path + ".type",
			Message: fmt.Sprintf(
				"unknown %s component %q (catalog covers v0.2 starter set only)",
				category, c.Type,
			),
		}}
	}

	var errs []ValidationError
	raw := c.Config.Raw

	if len(comp.CompositeFields) > 0 {
		isDirectArray := len(comp.CompositeFields) == 1 && comp.CompositeFields[0].Field == ""
		if !isDirectArray {
			// Pattern A: strip composite fields before validating scalar fields.
			errs = append(errs, validateConfig(path+".config", stripCompositeFields(raw, comp.CompositeFields), comp.ConfigSchema)...)
		}
		// Recursively validate every nested ComponentSpec in all composite fields.
		errs = append(errs, validateNestedComponents(path+".config", raw, comp.CompositeFields, cat)...)
	} else {
		errs = validateConfig(path+".config", raw, comp.ConfigSchema)
	}

	return errs
}

// validateNestedComponents recurses into composite fields and validates each
// nested ComponentSpec (type + config) against the catalog.
func validateNestedComponents(path string, raw []byte, fields []catalog.CompositeField, cat *catalog.Catalog) []ValidationError {
	var errs []ValidationError

	// nestedSpec is the wire format of a ComponentSpec inside a composite field.
	type nestedSpec struct {
		Type   string          `json:"type"`
		Config json.RawMessage `json:"config"`
	}

	parseItems := func(data []byte) ([]nestedSpec, bool) {
		var items []nestedSpec
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, false
		}
		return items, true
	}

	for _, cf := range fields {
		var itemBytes []byte
		if cf.Field == "" {
			// Pattern B: raw itself is the array.
			itemBytes = raw
		} else {
			// Pattern A: extract the named field from the config object.
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			itemBytes = m[cf.Field]
		}

		items, ok := parseItems(itemBytes)
		if !ok {
			continue
		}

		fieldPath := path
		if cf.Field != "" {
			fieldPath = path + "." + cf.Field
		}

		for i, item := range items {
			cs := &rpcv1alpha1.ComponentSpec{
				Type:   item.Type,
				Config: runtime.RawExtension{Raw: item.Config},
			}
			errs = append(errs, validateComponent(fmt.Sprintf("%s[%d]", fieldPath, i), cs, cf.Kind, cat)...)
		}
	}

	return errs
}

// stripCompositeFields removes composite sub-component fields from a JSON object so
// that configSchema (which only describes scalar fields) can validate the remainder.
func stripCompositeFields(raw []byte, fields []catalog.CompositeField) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // let validateConfig report the parse error
	}
	for _, cf := range fields {
		if cf.Field != "" {
			delete(m, cf.Field)
		}
	}
	stripped, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return stripped
}

func validateConfig(path string, raw []byte, schema json.RawMessage) []ValidationError {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}

	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return []ValidationError{{Path: path, Message: "schema parse: " + err.Error()}}
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(path, schemaDoc); err != nil {
		return []ValidationError{{Path: path, Message: "schema compile: " + err.Error()}}
	}
	sch, err := compiler.Compile(path)
	if err != nil {
		return []ValidationError{{Path: path, Message: "schema compile: " + err.Error()}}
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return []ValidationError{{Path: path, Message: "config is not valid JSON: " + err.Error()}}
	}
	if err := sch.Validate(instance); err != nil {
		return []ValidationError{{Path: path, Message: err.Error()}}
	}
	return nil
}
