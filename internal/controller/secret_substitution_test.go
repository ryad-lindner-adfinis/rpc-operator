package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// Ginkgo Describe block — runs inside the envtest suite (suite_test.go).
var _ = Describe("fetchSecretValues", func() {
	const namespace = "default"
	ctx := context.Background()

	It("resolves all refs from a single secret", func() {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "mysecret", Namespace: namespace},
			Data:       map[string][]byte{"password": []byte("s3cr3t"), "user": []byte("alice")},
		}
		Expect(k8sClient.Create(ctx, sec)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, sec) })

		refs := []rpcv1alpha1.SecretRef{
			{EnvVar: "DB_PASS", SecretName: "mysecret", Key: "password"},
			{EnvVar: "DB_USER", SecretName: "mysecret", Key: "user"},
		}
		vals, err := fetchSecretValues(ctx, k8sClient, namespace, refs)
		Expect(err).NotTo(HaveOccurred())
		Expect(vals).To(Equal(map[string]string{"DB_PASS": "s3cr3t", "DB_USER": "alice"}))
	})

	It("reads each secret only once when multiple refs use the same secretName", func() {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: namespace},
			Data:       map[string][]byte{"k1": []byte("v1"), "k2": []byte("v2")},
		}
		Expect(k8sClient.Create(ctx, sec)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, sec) })

		refs := []rpcv1alpha1.SecretRef{
			{EnvVar: "A", SecretName: "shared", Key: "k1"},
			{EnvVar: "B", SecretName: "shared", Key: "k2"},
		}
		vals, err := fetchSecretValues(ctx, k8sClient, namespace, refs)
		Expect(err).NotTo(HaveOccurred())
		Expect(vals).To(Equal(map[string]string{"A": "v1", "B": "v2"}))
	})

	It("returns an error when the secret does not exist", func() {
		refs := []rpcv1alpha1.SecretRef{
			{EnvVar: "X", SecretName: "no-such-secret", Key: "k"},
		}
		_, err := fetchSecretValues(ctx, k8sClient, namespace, refs)
		Expect(err).To(MatchError(ContainSubstring("no-such-secret")))
	})

	It("returns an error when the key is missing in the secret", func() {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: namespace},
			Data:       map[string][]byte{"other": []byte("val")},
		}
		Expect(k8sClient.Create(ctx, sec)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, sec) })

		refs := []rpcv1alpha1.SecretRef{
			{EnvVar: "MISSING", SecretName: "partial", Key: "no-such-key"},
		}
		_, err := fetchSecretValues(ctx, k8sClient, namespace, refs)
		Expect(err).To(MatchError(ContainSubstring("no-such-key")))
	})

	It("returns nil for empty refs", func() {
		vals, err := fetchSecretValues(ctx, k8sClient, namespace, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(vals).To(BeNil())
	})
})
