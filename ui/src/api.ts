import type { CatalogComponent, MetricsResponse, Pipeline, PipelineSpec, ValidateResponse } from './types'
import { getToken, clearToken } from './auth'

const BASE = '/api/v1'

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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
    window.dispatchEvent(new CustomEvent('rpc-auth-expired'))
  }
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw Object.assign(new Error(err.error ?? resp.statusText), { status: resp.status, body: err })
  }
  if (resp.status === 204) return undefined as T
  return resp.json()
}

export interface WhoamiResponse {
  user: { name: string; uid?: string; groups?: string[] }
  anonymous: boolean
}

export async function whoami(): Promise<WhoamiResponse> {
  return request<WhoamiResponse>('GET', '/auth/whoami')
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

export async function getMetrics(
  namespace: string,
  name: string,
  query: 'throughput' | 'error_rate' | 'input_rate' | 'processor_error_rate',
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
