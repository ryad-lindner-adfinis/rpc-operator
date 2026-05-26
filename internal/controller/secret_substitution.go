package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rpcv1alpha1 "github.com/insidegreen/rpc-operator-claude/api/v1alpha1"
)

// varPattern matches ${VARNAME} and ${VARNAME:any_existing_default}.
// Bloblang expressions like ${!...} do not match because '!' is not [A-Za-z_].
var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::[^}]*)?\}`)

// substituteSecrets rewrites ${ENVVAR} and ${ENVVAR:old_default} to ${ENVVAR:actual_value}
// for every envVar listed in refs whose name appears as a key in values.
// Variables not present in values and Bloblang expressions are left unchanged.
// values is built by fetchSecretValues and discarded after the stream PUT — never logged.
func substituteSecrets(yamlText string, refs []rpcv1alpha1.SecretRef, values map[string]string) string {
	if len(refs) == 0 || len(values) == 0 {
		return yamlText
	}
	return varPattern.ReplaceAllStringFunc(yamlText, func(match string) string {
		// match is "${VARNAME}" or "${VARNAME:old}"; strip ${ and }
		inner := match[2 : len(match)-1]
		name := inner
		if idx := strings.IndexByte(inner, ':'); idx >= 0 {
			name = inner[:idx]
		}
		val, ok := values[name]
		if !ok {
			return match
		}
		return "${" + name + ":" + val + "}"
	})
}

// fetchSecretValues reads the K8s Secrets referenced in refs, extracts the
// specified keys, and returns a map of envVar → actual_value.
// Each unique secretName is fetched exactly once. Returns an error if any
// secret or key is missing — the caller should mark the pipeline SecretNotFound.
// The returned map must not be logged (it contains secret values).
func fetchSecretValues(ctx context.Context, c client.Client,
	namespace string, refs []rpcv1alpha1.SecretRef) (map[string]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	secrets := make(map[string]*corev1.Secret, len(refs))
	for _, ref := range refs {
		if _, already := secrets[ref.SecretName]; already {
			continue
		}
		var s corev1.Secret
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.SecretName}, &s); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("secret %q not found", ref.SecretName)
			}
			return nil, err
		}
		secrets[ref.SecretName] = &s
	}
	values := make(map[string]string, len(refs))
	for _, ref := range refs {
		raw, ok := secrets[ref.SecretName].Data[ref.Key]
		if !ok {
			return nil, fmt.Errorf("key %q not found in secret %q", ref.Key, ref.SecretName)
		}
		values[ref.EnvVar] = string(raw)
	}
	return values, nil
}
