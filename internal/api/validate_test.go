package api_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
	"github.com/insidegreen/rpc-operator-claude/internal/api"
	"github.com/insidegreen/rpc-operator-claude/internal/api/catalog"
)

func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return cat
}

func pipelineWith(
	inputType string, inputConfig []byte,
	procType string, procConfig []byte,
	outputType string,
) *rpcv1alpha1.Pipeline {
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input: rpcv1alpha1.ComponentSpec{
				Type:   inputType,
				Config: runtime.RawExtension{Raw: inputConfig},
			},
			Output: rpcv1alpha1.ComponentSpec{Type: outputType},
		},
	}
	if procType != "" {
		p.Spec.Processors = []rpcv1alpha1.ComponentSpec{{
			Type:   procType,
			Label:  "test",
			Config: runtime.RawExtension{Raw: procConfig},
		}}
	}
	return p
}

func TestValidate_HappyPath(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\"","interval":"1s","count":3}`),
		"mapping", []byte(`"root = content().uppercase()"`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_UnknownInputComponent(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith("no-such-input", nil, "", nil, "stdout")
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error for unknown input component")
	}
	if errs[0].Path != "spec.input.type" {
		t.Errorf("expected path spec.input.type, got %q", errs[0].Path)
	}
}

func TestValidate_UnknownOutputComponent(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\""}`),
		"", nil,
		"no-such-output",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error for unknown output component")
	}
	found := false
	for _, e := range errs {
		if e.Path == "spec.output.type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error path spec.output.type, got %v", errs)
	}
}

// TestValidate_ScalarBodyAsObject verifies the v0.1 smoke-test bug is caught:
// the mapping processor requires a scalar string config, not an object.
func TestValidate_ScalarBodyAsObject(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\""}`),
		"mapping", []byte(`{"mapping":"root = content().uppercase()"`+`}`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error: object body where scalar expected")
	}
	found := false
	for _, e := range errs {
		if e.Path == "spec.processors[0].config" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at spec.processors[0].config, got %v", errs)
	}
}

func TestValidate_ObjectBodyAsScalar(t *testing.T) {
	cat := mustLoadCatalog(t)
	// generate requires an object config — sending a string should fail
	p := pipelineWith("generate", []byte(`"oops"`), "", nil, "stdout")
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error: scalar body where object expected")
	}
}

func TestValidate_MissingRequiredField(t *testing.T) {
	cat := mustLoadCatalog(t)
	// generate requires "mapping" field
	p := pipelineWith("generate", []byte(`{"interval":"1s"}`), "", nil, "stdout")
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error: missing required 'mapping' field in generate config")
	}
}

func TestValidate_RenderFailure(t *testing.T) {
	cat := mustLoadCatalog(t)
	// Invalid JSON in RawExtension causes render to fail
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			Input: rpcv1alpha1.ComponentSpec{
				Type:   "generate",
				Config: runtime.RawExtension{Raw: []byte(`{not valid json`)},
			},
			Output: rpcv1alpha1.ComponentSpec{Type: "stdout"},
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation error from render failure")
	}
}

func TestValidate_EmptyProcessors(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\""}`),
		"", nil,
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors with no processors, got %v", errs)
	}
}

func TestValidate_BranchComposite(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\"","count":1}`),
		"branch", []byte(`{
			"request_map": "root = this",
			"processors": [{"type": "mapping", "config": "root = content().uppercase()"}],
			"result_map": "root = this"
		}`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors for branch composite, got %v", errs)
	}
}

func TestValidate_ForEachComposite(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\"","count":1}`),
		"for_each", []byte(`[{"type":"mapping","config":"root = content().uppercase()"}]`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors for for_each composite, got %v", errs)
	}
}

func TestValidate_BranchUnknownNestedProcessor(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\"","count":1}`),
		"branch", []byte(`{
			"request_map": "root = this",
			"processors": [{"type": "no-such-processor", "config": {}}],
			"result_map": "root = this"
		}`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown nested processor type")
	}
	found := false
	for _, e := range errs {
		if e.Path == "spec.processors[0].config.processors[0].type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at nested processor path, got %v", errs)
	}
}

func TestValidate_ForEachUnknownNestedProcessor(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\"","count":1}`),
		"for_each", []byte(`[{"type":"no-such-processor","config":{}}]`),
		"stdout",
	)
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown nested processor in for_each")
	}
	found := false
	for _, e := range errs {
		if e.Path == "spec.processors[0].config[0].type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error at nested for_each path, got %v", errs)
	}
}

func TestValidate_NullConfig(t *testing.T) {
	cat := mustLoadCatalog(t)
	// stdout accepts an empty/null config
	p := pipelineWith(
		"generate", []byte(`{"mapping":"root = \"hi\""}`),
		"", nil,
		"stdout",
	)
	p.Spec.Output.Config = runtime.RawExtension{Raw: []byte("null")}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors for null stdout config, got %v", errs)
	}
}

func TestValidate_MissingProcessorLabel(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := pipelineWith("generate", []byte(`{"mapping":"root = \"hi\""}`),
		"mapping", []byte(`"root = content().uppercase()"`), "stdout")
	p.Spec.Processors[0].Label = "" // explicitly clear the default "test" label
	errs := api.ValidatePipeline(p, cat)
	found := false
	for _, e := range errs {
		if e.Path == "spec.processors[0].label" {
			found = true
		}
	}
	if !found {
		t.Error("expected ValidationError for missing processor label")
	}
}

func TestValidate_RawYAML_Valid(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "raw-test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  generate:\n    mapping: 'root = \"hi\"'\n    interval: 1s\noutput:\n  stdout: {}\n",
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid rawYAML, got %v", errs)
	}
}

func TestValidate_RawYAML_InvalidYAML(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "raw-test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "{invalid: yaml: [",
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected ValidationError for invalid YAML")
	}
	if errs[0].Path != "spec.rawYAML" {
		t.Errorf("expected path spec.rawYAML, got %q", errs[0].Path)
	}
}

func TestValidate_RawYAML_NotAMapping(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "raw-test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "- item1\n- item2\n",
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Fatal("expected ValidationError for non-mapping YAML")
	}
	if errs[0].Path != "spec.rawYAML" {
		t.Errorf("expected path spec.rawYAML, got %q", errs[0].Path)
	}
}

func TestValidate_RawYAML_SkipsCatalogValidation(t *testing.T) {
	cat := mustLoadCatalog(t)
	// Uses a component type that is NOT in the catalog — should pass in raw mode.
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "raw-test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  kafka_franz:\n    seed_brokers: [broker:9092]\noutput:\n  stdout: {}\n",
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("catalog validation should be skipped in raw mode, got %v", errs)
	}
}

func TestValidateSecretRefs_Valid(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}",
			SecretRefs: []rpcv1alpha1.SecretRef{
				{EnvVar: "MY_PASSWORD", SecretName: "my-secret", Key: "password"},
			},
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid secretRef, got %v", errs)
	}
}

func TestValidateSecretRefs_EmptyFields(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}",
			SecretRefs: []rpcv1alpha1.SecretRef{
				{EnvVar: "", SecretName: "", Key: ""},
			},
		},
	}
	errs := api.ValidatePipeline(p, cat)
	paths := make(map[string]bool)
	for _, e := range errs {
		paths[e.Path] = true
	}
	for _, want := range []string{
		"spec.secretRefs[0].envVar",
		"spec.secretRefs[0].secretName",
		"spec.secretRefs[0].key",
	} {
		if !paths[want] {
			t.Errorf("expected error at path %q, got errors: %v", want, errs)
		}
	}
}

func TestValidateSecretRefs_InvalidEnvVar(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}",
			SecretRefs: []rpcv1alpha1.SecretRef{
				{EnvVar: "123_INVALID", SecretName: "s", Key: "k"},
			},
		},
	}
	errs := api.ValidatePipeline(p, cat)
	if len(errs) == 0 {
		t.Error("expected error for invalid envVar name, got none")
	}
	if errs[0].Path != "spec.secretRefs[0].envVar" {
		t.Errorf("expected path spec.secretRefs[0].envVar, got %q", errs[0].Path)
	}
}

func TestValidateSecretRefs_Duplicate(t *testing.T) {
	cat := mustLoadCatalog(t)
	p := &rpcv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: rpcv1alpha1.PipelineSpec{
			RawYAML: "input:\n  stdin: {}\noutput:\n  stdout: {}",
			SecretRefs: []rpcv1alpha1.SecretRef{
				{EnvVar: "MY_VAR", SecretName: "s1", Key: "k1"},
				{EnvVar: "MY_VAR", SecretName: "s2", Key: "k2"},
			},
		},
	}
	errs := api.ValidatePipeline(p, cat)
	found := false
	for _, e := range errs {
		if e.Path == "spec.secretRefs[1].envVar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate envVar error at spec.secretRefs[1].envVar, got: %v", errs)
	}
}
