import { describe, it, expect, afterEach, beforeAll, afterAll } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { RawPipelineEditor } from './RawPipelineEditor'
import type { Pipeline, PipelineProject } from '../types'

const orders: PipelineProject = {
  metadata: { name: 'orders', namespace: 'default' },
  spec: { routes: [] },
}
const other: PipelineProject = {
  metadata: { name: 'other', namespace: 'default' },
  spec: { routes: [] },
}

const server = setupServer(
  http.get('/api/v1/namespaces/default/pipelineclusters', () =>
    HttpResponse.json({ items: [] })),
  http.get('/api/v1/namespaces/default/pipelineprojects', () =>
    HttpResponse.json({ items: [orders, other] })),
)
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('RawPipelineEditor', () => {
  it('pre-selects the project from initialProjectRef for a new pipeline', async () => {
    render(<RawPipelineEditor namespace="default" initialProjectRef="orders"
      onBack={() => {}} onSaved={() => {}} />)
    await screen.findByRole('option', { name: 'orders' })
    const projectSelect = screen.getByRole('combobox', { name: /Project/i }) as HTMLSelectElement
    expect(projectSelect.value).toBe('orders')
  })

  it('lets an editPipeline projectRef win over initialProjectRef', async () => {
    const editPipeline: Pipeline = {
      apiVersion: 'rpc.operator.io/v1alpha1',
      kind: 'Pipeline',
      metadata: { name: 'p1', namespace: 'default', resourceVersion: '7' },
      spec: { rawYAML: 'input: {}', projectRef: { name: 'orders' } },
    }
    render(<RawPipelineEditor namespace="default" editPipeline={editPipeline}
      initialProjectRef="other" onBack={() => {}} onSaved={() => {}} />)
    await screen.findByRole('option', { name: 'orders' })
    const projectSelect = screen.getByRole('combobox', { name: /Project/i }) as HTMLSelectElement
    expect(projectSelect.value).toBe('orders')
  })

  it('deploys a new pipeline with the pre-filled projectRef in the payload', async () => {
    let captured: unknown = null
    server.use(http.post('/api/v1/namespaces/default/pipelines', async ({ request }) => {
      captured = await request.json()
      return HttpResponse.json({ metadata: { name: 'np', namespace: 'default' }, spec: {} })
    }))

    render(<RawPipelineEditor namespace="default" initialProjectRef="orders"
      onBack={() => {}} onSaved={() => {}} />)
    await screen.findByRole('option', { name: 'orders' })

    await userEvent.type(screen.getByRole('textbox', { name: /Pipeline name/i }), 'np')
    await userEvent.click(screen.getByRole('button', { name: /Deploy/i }))

    await waitFor(() => expect(captured).not.toBeNull())
    expect((captured as { spec: { projectRef?: { name: string } } }).spec.projectRef)
      .toEqual({ name: 'orders' })
  })
})
