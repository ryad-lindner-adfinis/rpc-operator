package controller

import (
	"testing"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

func TestSubstituteSecrets(t *testing.T) {
	refs := []rpcv1alpha1.SecretRef{{EnvVar: "DB_PASS", SecretName: "s", Key: "k"}}
	cases := []struct {
		name   string
		input  string
		values map[string]string
		want   string
	}{
		{
			name:   "single var substituted",
			input:  `url: "http://user:${DB_PASS}@host"`,
			values: map[string]string{"DB_PASS": "s3cr3t"},
			want:   `url: "http://user:${DB_PASS:s3cr3t}@host"`,
		},
		{
			name:   "replaces existing default",
			input:  `${DB_PASS:old_default}`,
			values: map[string]string{"DB_PASS": "new_value"},
			want:   `${DB_PASS:new_value}`,
		},
		{
			name:   "unknown var left untouched",
			input:  `${OTHER_VAR}`,
			values: map[string]string{"DB_PASS": "s3cr3t"},
			want:   `${OTHER_VAR}`,
		},
		{
			name:   "empty values map returns input unchanged",
			input:  `${DB_PASS}`,
			values: map[string]string{},
			want:   `${DB_PASS}`,
		},
		{
			name:   "multiple vars substituted",
			input:  `url: ${HOST} pass: ${DB_PASS}`,
			values: map[string]string{"DB_PASS": "s3cr3t", "HOST": "myhost"},
			want:   `url: ${HOST:myhost} pass: ${DB_PASS:s3cr3t}`,
		},
		{
			name:   "bloblang ${!expr} left untouched because ! does not match [A-Za-z_]",
			input:  `${!this.field}`,
			values: map[string]string{"!this.field": "x"},
			want:   `${!this.field}`,
		},
		{
			name:   "empty yaml returns empty",
			input:  ``,
			values: map[string]string{"DB_PASS": "s3cr3t"},
			want:   ``,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := substituteSecrets(tc.input, refs, tc.values)
			if got != tc.want {
				t.Errorf("substituteSecrets(%q)\ngot  %q\nwant %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSubstituteSecretsEmptyRefs(t *testing.T) {
	// With nil/empty refs the function must return the input unchanged regardless of values.
	input := `${DB_PASS}`
	got := substituteSecrets(input, nil, map[string]string{"DB_PASS": "s3cr3t"})
	if got != input {
		t.Errorf("expected unchanged input with nil refs, got %q", got)
	}
}
