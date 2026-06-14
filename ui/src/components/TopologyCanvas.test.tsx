import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TopologyCanvas } from './TopologyCanvas'
import { buildTopology, computeLayout } from '../topology'
import type { PipelineProject } from '../types'

const project: PipelineProject = {
  metadata: { name: 'orders', namespace: 'default' },
  spec: { routes: [{ name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] }] },
}

describe('TopologyCanvas', () => {
  it('renders one labelled box per node', () => {
    const topo = computeLayout(buildTopology(project))
    render(<TopologyCanvas topology={topo} selectedId={null} onSelect={() => {}} />)
    expect(screen.getByText('ingest')).toBeInTheDocument()
    expect(screen.getByText('warehouse')).toBeInTheDocument()
    expect(screen.getByText('fan')).toBeInTheDocument()       // router pill label
  })

  it('marks the selected node', () => {
    const topo = computeLayout(buildTopology(project))
    const { container } = render(
      <TopologyCanvas topology={topo} selectedId="ingest" onSelect={() => {}} />)
    expect(container.querySelector('[data-selected="true"]')).toBeTruthy()
  })
})

describe('TopologyCanvas caches', () => {
  const withCache: PipelineProject = {
    metadata: { name: 'orders', namespace: 'default' },
    spec: {
      routes: [{ name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] }],
      cacheResources: [{ name: 'shared', natsKV: {} }],
    },
  }

  it('renders the cache node label and the operator label on its edge', () => {
    const topo = computeLayout(buildTopology(withCache, ['ingest', 'warehouse'],
      [{ pipeline: 'warehouse', cache: 'shared', operators: ['get', 'set'] }]))
    const { container } = render(<TopologyCanvas topology={topo} selectedId={null} onSelect={() => {}} />)
    expect(screen.getByText('shared')).toBeInTheDocument()
    expect(screen.getByText('get, set')).toBeInTheDocument()
    // dashed cache edge present
    expect(container.querySelector('path[stroke-dasharray]')).toBeTruthy()
  })
})
