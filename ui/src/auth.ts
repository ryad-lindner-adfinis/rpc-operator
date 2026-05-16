import yaml from 'js-yaml'

const TOKEN_KEY = 'rpc-operator-token'

export function getToken(): string | null {
  return sessionStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  sessionStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  sessionStorage.removeItem(TOKEN_KEY)
}

/** Minimal kubeconfig shape we care about. */
interface KubeconfigUser {
  name?: string
  user: {
    token?: string
    'client-certificate-data'?: string
    'client-key-data'?: string
  }
}
interface Kubeconfig {
  users?: KubeconfigUser[]
}

/**
 * Parse a kubeconfig text and extract the first user's Bearer token.
 * Rejects cert-auth kubeconfigs with a clear message — the API server
 * needs a token, not a client certificate.
 */
export function parseKubeconfigToken(text: string): { token: string } | { error: string } {
  let doc: Kubeconfig
  try {
    doc = yaml.load(text) as Kubeconfig
  } catch (e) {
    return { error: `YAML parse error: ${(e as Error).message}` }
  }
  if (!doc || !Array.isArray(doc.users) || doc.users.length === 0) {
    return { error: 'kubeconfig has no users entry' }
  }
  const first = doc.users[0]
  if (!first.user) return { error: 'first user has no user block' }
  if (first.user.token) return { token: first.user.token }
  if (first.user['client-certificate-data']) {
    return {
      error:
        'Cert-Auth kubeconfig is not supported. Use `kubectl create token <sa>` to get a Bearer token.',
    }
  }
  return { error: 'no token in kubeconfig' }
}
