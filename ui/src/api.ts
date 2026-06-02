import type {
  CatalogComponent, ClusterDistribution, MetricQuery, MetricsResponse,
  Pipeline, PipelineCluster, PipelineClusterSpec, PipelineProject, PipelineProjectSpec,
  PipelineSpec, ValidateResponse,
} from './types'
import { getToken, clearToken } from './auth'

const BASE = '/api/v1'

export async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts?: { suppressAuthExpired?: boolean },
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const token = getToken()
  if (token) headers['Authorization'] = 'Bearer ' + token
  const resp = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (resp.status === 401) {
    clearToken()
    // rpc-auth-expired re-resolves auth via whoami. whoami itself must NOT fire
    // it — its 401 is the terminal "not authenticated" signal handled by the
    // caller's .catch; re-dispatching here loops whoami → onExpire → whoami.
    if (!opts?.suppressAuthExpired) {
      window.dispatchEvent(new CustomEvent('rpc-auth-expired'))
    }
  }
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    const msg = err.details ? `${err.error ?? resp.statusText}: ${err.details}` : (err.error ?? resp.statusText)
    throw Object.assign(new Error(msg), { status: resp.status, body: err })
  }
  if (resp.status === 204) return undefined as T
  return resp.json()
}

export interface WhoamiResponse {
  user: { name: string; uid?: string; groups?: string[] }
  anonymous: boolean
  /** F42: true in Mode C (anonymous read-only). UI hides write actions. */
  readOnly: boolean
  /** F42: server's anonymous.logs.enabled. Only relevant when anonymous=true. */
  anonymousLogs: boolean
  /** F20b: backend has an OIDC issuer configured. UI shows the SSO button when true. */
  oidcEnabled: boolean
}

export async function whoami(): Promise<WhoamiResponse> {
  return request<WhoamiResponse>('GET', '/auth/whoami', undefined, { suppressAuthExpired: true })
}

// F20b: token-free capabilities probe. Lets the login screen show the SSO
// button in Mode B strict, where whoami 401s before the user has a token.
export async function authConfig(): Promise<{ oidcEnabled: boolean; visualEditorEnabled: boolean }> {
  return request<{ oidcEnabled: boolean; visualEditorEnabled: boolean }>('GET', '/auth/config')
}

// F20b: exchanges the backend-cached refresh_token for a fresh id_token.
// Carries the OIDC session cookie via credentials: include. Throws on failure
// (no session, IdP rejection, etc.) so callers can fall back to a fresh login.
export async function refreshOIDC(): Promise<string> {
  const resp = await fetch(BASE + '/auth/refresh', {
    method: 'POST',
    credentials: 'include',
  })
  if (!resp.ok) {
    throw Object.assign(new Error('refresh failed: ' + resp.status), { status: resp.status })
  }
  const data = (await resp.json()) as { id_token: string }
  if (!data.id_token) throw new Error('refresh response missing id_token')
  return data.id_token
}

// F20b: best-effort backend logout — removes the cached refresh_token and
// expires the session cookie. Never throws; UI logout proceeds regardless.
export async function oidcLogout(): Promise<void> {
  try {
    await fetch(BASE + '/auth/logout', { method: 'POST', credentials: 'include' })
  } catch {
    // swallow — UI clears its local token next, which is what really matters
  }
}

export async function listCatalog(): Promise<CatalogComponent[]> {
  const data = await request<{ items: CatalogComponent[] }>('GET', '/catalog')
  return data.items
}

export async function listNamespaces(): Promise<string[]> {
  const data = await request<{ namespaces: string[] }>('GET', '/namespaces')
  return data.namespaces ?? []
}

export async function getPipeline(namespace: string, name: string): Promise<Pipeline> {
  return request<Pipeline>('GET', `/namespaces/${namespace}/pipelines/${name}`)
}

export async function validatePipeline(
  namespace: string,
  name: string,
  spec: PipelineSpec,
): Promise<ValidateResponse> {
  return request<ValidateResponse>('POST', '/pipelines/validate', {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'Pipeline',
    metadata: { name, namespace },
    spec,
  })
}

export async function renderPipelineYAML(
  namespace: string,
  name: string,
  spec: PipelineSpec,
): Promise<string> {
  const resp = await request<{ yaml: string }>('POST', '/pipelines/render', {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'Pipeline',
    metadata: { name, namespace },
    spec,
  })
  return resp.yaml
}

export async function createPipeline(
  namespace: string,
  name: string,
  spec: PipelineSpec,
): Promise<Pipeline> {
  return request<Pipeline>('POST', `/namespaces/${namespace}/pipelines`, {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'Pipeline',
    metadata: { name, namespace },
    spec,
  })
}

export async function updatePipeline(
  namespace: string,
  name: string,
  spec: PipelineSpec,
  resourceVersion?: string,
): Promise<Pipeline> {
  return request<Pipeline>('PUT', `/namespaces/${namespace}/pipelines/${name}`, {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'Pipeline',
    metadata: { name, namespace, ...(resourceVersion ? { resourceVersion } : {}) },
    spec,
  })
}

export async function listPipelines(namespace: string): Promise<Pipeline[]> {
  const data = await request<{ items: Pipeline[] }>('GET', `/namespaces/${namespace}/pipelines`)
  return data.items ?? []
}

export async function deletePipeline(namespace: string, name: string): Promise<void> {
  await request<void>('DELETE', `/namespaces/${namespace}/pipelines/${name}`)
}

// F45: stop and run subresources. Idempotent on both sides.
export async function stopPipeline(namespace: string, name: string): Promise<Pipeline> {
  return request<Pipeline>('POST', `/namespaces/${namespace}/pipelines/${name}/stop`)
}

export async function runPipeline(namespace: string, name: string): Promise<Pipeline> {
  return request<Pipeline>('POST', `/namespaces/${namespace}/pipelines/${name}/run`)
}

export async function getMetrics(
  namespace: string,
  name: string,
  query: MetricQuery,
  startSec: number,
  endSec: number,
  step = '30s',
): Promise<MetricsResponse> {
  const params = new URLSearchParams({
    query,
    start: String(startSec),
    end: String(endSec),
    step,
  })
  return request<MetricsResponse>(
    'GET',
    `/namespaces/${namespace}/pipelines/${name}/metrics?${params}`,
  )
}

// --- F47 Phase 3c: PipelineCluster client ---

export async function listClusters(namespace: string): Promise<PipelineCluster[]> {
  const data = await request<{ items: PipelineCluster[] }>(
    'GET', `/namespaces/${namespace}/pipelineclusters`,
  )
  return data.items ?? []
}

export async function getCluster(namespace: string, name: string): Promise<PipelineCluster> {
  return request<PipelineCluster>('GET', `/namespaces/${namespace}/pipelineclusters/${name}`)
}

export async function getClusterInstances(
  namespace: string,
  name: string,
): Promise<ClusterDistribution> {
  return request<ClusterDistribution>(
    'GET', `/namespaces/${namespace}/pipelineclusters/${name}/instances`,
  )
}

export async function getClusterMetrics(
  namespace: string,
  name: string,
  query: MetricQuery,
  startSec: number,
  endSec: number,
  step = '30s',
): Promise<MetricsResponse> {
  const params = new URLSearchParams({
    query,
    start: String(startSec),
    end: String(endSec),
    step,
  })
  return request<MetricsResponse>(
    'GET',
    `/namespaces/${namespace}/pipelineclusters/${name}/metrics?${params}`,
  )
}

// The Phase-3b PUT replaces .spec wholesale, so callers must send the full spec
// (read it via getCluster first, then mutate the field they want).
export async function updateCluster(
  namespace: string,
  name: string,
  spec: PipelineClusterSpec,
  resourceVersion?: string,
): Promise<PipelineCluster> {
  return request<PipelineCluster>('PUT', `/namespaces/${namespace}/pipelineclusters/${name}`, {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'PipelineCluster',
    metadata: { name, namespace, ...(resourceVersion ? { resourceVersion } : {}) },
    spec,
  })
}

// --- F50.3: PipelineProject client ---

export async function listProjects(namespace: string): Promise<PipelineProject[]> {
  const data = await request<{ items: PipelineProject[] }>(
    'GET', `/namespaces/${namespace}/pipelineprojects`,
  )
  return data.items ?? []
}

export async function getProject(namespace: string, name: string): Promise<PipelineProject> {
  return request<PipelineProject>('GET', `/namespaces/${namespace}/pipelineprojects/${name}`)
}

export async function createProject(
  namespace: string,
  name: string,
  spec: PipelineProjectSpec,
): Promise<PipelineProject> {
  return request<PipelineProject>('POST', `/namespaces/${namespace}/pipelineprojects`, {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'PipelineProject',
    metadata: { name, namespace },
    spec,
  })
}

// PUT replaces .spec wholesale (mirror updateCluster): read via getProject,
// mutate the field, then send the full spec back.
export async function updateProject(
  namespace: string,
  name: string,
  spec: PipelineProjectSpec,
  resourceVersion?: string,
): Promise<PipelineProject> {
  return request<PipelineProject>('PUT', `/namespaces/${namespace}/pipelineprojects/${name}`, {
    apiVersion: 'rpc.operator.io/v1alpha1',
    kind: 'PipelineProject',
    metadata: { name, namespace, ...(resourceVersion ? { resourceVersion } : {}) },
    spec,
  })
}

export async function deleteProject(namespace: string, name: string): Promise<void> {
  await request<void>('DELETE', `/namespaces/${namespace}/pipelineprojects/${name}`)
}
