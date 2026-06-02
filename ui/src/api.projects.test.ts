import { describe, it, expect, afterEach, beforeAll, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { listProjects, getProject, createProject, deleteProject } from './api'
import type { PipelineProject } from './types'

const orders: PipelineProject = {
  metadata: { name: 'orders', namespace: 'default' },
  spec: { description: 'routed', routes: [{ name: 'ingest', from: 'a', to: [{ pipeline: 'b' }] }] },
  status: { phase: 'Ready', cluster: { name: 'orders-cluster', ready: 1, total: 1 } },
}

const server = setupServer(
  http.get('/api/v1/namespaces/default/pipelineprojects', () =>
    HttpResponse.json({ items: [orders] })),
  http.get('/api/v1/namespaces/default/pipelineprojects/orders', () =>
    HttpResponse.json(orders)),
  http.post('/api/v1/namespaces/default/pipelineprojects', async ({ request }) => {
    const body = (await request.json()) as PipelineProject
    return HttpResponse.json(body, { status: 201 })
  }),
  http.delete('/api/v1/namespaces/default/pipelineprojects/orders', () =>
    HttpResponse.json({ deleted: 'orders' })),
)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('project api client', () => {
  it('lists projects', async () => {
    const got = await listProjects('default')
    expect(got).toHaveLength(1)
    expect(got[0].metadata.name).toBe('orders')
  })

  it('gets a project', async () => {
    const got = await getProject('default', 'orders')
    expect(got.status?.cluster?.name).toBe('orders-cluster')
  })

  it('creates a project with the right envelope', async () => {
    const created = await createProject('default', 'neo', { description: 'x' })
    expect(created.metadata.name).toBe('neo')
    expect(created.kind).toBe('PipelineProject')
  })

  it('deletes a project', async () => {
    await expect(deleteProject('default', 'orders')).resolves.toBeUndefined()
  })
})
