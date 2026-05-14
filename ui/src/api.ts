import type { CatalogComponent, Pipeline, PipelineSpec, ValidateResponse } from './types'

const BASE = '/api/v1'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(BASE + path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw Object.assign(new Error(err.error ?? resp.statusText), { status: resp.status, body: err })
  }
  if (resp.status === 204) return undefined as T
  return resp.json()
}

export async function listCatalog(): Promise<CatalogComponent[]> {
  const data = await request<{ items: CatalogComponent[] }>('GET', '/catalog')
  return data.items
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
