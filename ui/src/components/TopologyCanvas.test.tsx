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
