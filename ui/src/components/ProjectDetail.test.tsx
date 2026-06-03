import { describe, it, expect, afterEach, beforeAll, afterAll, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { ProjectDetail } from './ProjectDetail'
import type { PipelineProject } from '../types'

const orders: PipelineProject = {
  metadata: { name: 'orders', namespace: 'default' },
  spec: { routes: [{ name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] }] },
  status: { phase: 'Ready', cluster: { name: 'orders-cluster', ready: 1, total: 1 } },
}

const server = setupServer(
  http.get('/api/v1/namespaces/default/pipelineprojects/orders', () => HttpResponse.json(orders)),
)
beforeAll(() => server.listen({ onUnhandledRequest: 'bypass' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('ProjectDetail', () => {
  it('loads and renders the topology + side panel for a selected node', async () => {
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('ingest')).toBeInTheDocument())
    await userEvent.click(screen.getByText('fan'))               // select the router node
    expect(screen.getByText(/Subject/i)).toBeInTheDocument()     // router side panel
    expect(screen.getByText('rpc.orders.fan')).toBeInTheDocument()
  })

  it('surfaces a Degraded project condition as a banner', async () => {
    const degraded: PipelineProject = {
      ...orders,
      status: {
        phase: 'Degraded',
        conditions: [
          { type: 'RoutesValid', status: 'False', reason: 'InvalidRoutes',
            message: "input is managed by the project's routes; remove it" },
        ],
      },
    }
    server.use(
      http.get('/api/v1/namespaces/default/pipelineprojects/orders', () => HttpResponse.json(degraded)),
    )
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('Project degraded')).toBeInTheDocument())
    expect(screen.getByText(/input is managed by the project's routes/i)).toBeInTheDocument()
  })

  it('stages a router removal locally without deploying', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    let putHit = false
    server.use(
      http.put('/api/v1/namespaces/default/pipelineprojects/orders', () => {
        putHit = true
        return HttpResponse.json(orders)
      }),
    )
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('fan')).toBeInTheDocument())

    await userEvent.click(screen.getByText('fan'))                  // select router node
    await userEvent.click(screen.getByRole('button', { name: /Remove from draft/i }))

    // No deploy happened, the draft is dirty, and the node is gone from the map.
    expect(putHit).toBe(false)
    expect(screen.getByText(/Unsaved changes/i)).toBeInTheDocument()
    expect(screen.queryByText('fan')).toBeNull()
  })

  it('Save deploys the full draft once and clears the dirty pill', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    let putBody: any = null
    let server_routes = orders.spec.routes
    server.use(
      http.get('/api/v1/namespaces/default/pipelineprojects/orders', () =>
        HttpResponse.json({ ...orders, spec: { ...orders.spec, routes: server_routes } })),
      http.put('/api/v1/namespaces/default/pipelineprojects/orders', async ({ request }) => {
        putBody = await request.json()
        server_routes = putBody.spec.routes               // reflect the commit on next GET
        return HttpResponse.json({ ...orders, spec: { ...orders.spec, routes: server_routes } })
      }),
    )
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('fan')).toBeInTheDocument())

    await userEvent.click(screen.getByText('fan'))
    await userEvent.click(screen.getByRole('button', { name: /Remove from draft/i }))
    await userEvent.click(screen.getByRole('button', { name: /Save & deploy/i }))

    await waitFor(() => expect(screen.queryByText(/Unsaved changes/i)).toBeNull())
    expect(putBody.spec.routes).toEqual([])               // 'fan' removed
  })

  it('shows backend validation errors on a 422 and keeps the draft', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    server.use(
      http.put('/api/v1/namespaces/default/pipelineprojects/orders', () =>
        HttpResponse.json(
          { error: 'validation failed', errors: [{ path: 'spec.routes', message: "input is managed by the project's routes; remove it" }] },
          { status: 422 },
        )),
    )
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('fan')).toBeInTheDocument())

    await userEvent.click(screen.getByText('fan'))
    await userEvent.click(screen.getByRole('button', { name: /Remove from draft/i }))
    await userEvent.click(screen.getByRole('button', { name: /Save & deploy/i }))

    await waitFor(() => expect(screen.getByText(/Cannot deploy — fix these routes/i)).toBeInTheDocument())
    expect(screen.getByText(/input is managed by the project's routes/i)).toBeInTheDocument()
    expect(screen.getByText(/Unsaved changes/i)).toBeInTheDocument()   // still dirty
  })

  it('Discard reverts the draft to the server routes', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(<ProjectDetail namespace="default" name="orders" readOnly={false}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('fan')).toBeInTheDocument())

    await userEvent.click(screen.getByText('fan'))
    await userEvent.click(screen.getByRole('button', { name: /Remove from draft/i }))
    expect(screen.queryByText('fan')).toBeNull()

    await userEvent.click(screen.getByRole('button', { name: /Discard/i }))
    expect(screen.getByText('fan')).toBeInTheDocument()
    expect(screen.queryByText(/Unsaved changes/i)).toBeNull()
  })

  it('hides + Router in read-only mode', async () => {
    render(<ProjectDetail namespace="default" name="orders" readOnly={true}
      onBack={() => {}} onOpenPipeline={() => {}} onAddPipeline={() => {}} />)
    await waitFor(() => expect(screen.getByText('ingest')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /\+ Router/i })).toBeNull()
  })
})
