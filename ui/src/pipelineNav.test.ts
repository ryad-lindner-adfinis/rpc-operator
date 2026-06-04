import { describe, it, expect } from 'vitest'
import { pipelineBackTarget, editorBackTarget } from './pipelineNav'

describe('pipelineBackTarget', () => {
  it('routes a project origin back to the project detail', () => {
    expect(pipelineBackTarget({ kind: 'project', name: 'orders' }))
      .toEqual({ section: 'projects', projectsView: 'detail' })
  })

  it('routes a cluster origin back to the cluster detail', () => {
    expect(pipelineBackTarget({ kind: 'cluster', name: 'c1' }))
      .toEqual({ section: 'clusters', clustersView: 'detail' })
  })

  it('routes the default (pipeline-list) origin back to the pipeline list', () => {
    expect(pipelineBackTarget({ kind: 'pipelines' }))
      .toEqual({ section: 'pipelines', pipelinesView: 'list' })
  })
})

describe('editorBackTarget', () => {
  it('routes a detail-origin editor back to the pipeline detail', () => {
    expect(editorBackTarget({ kind: 'detail' }))
      .toEqual({ section: 'pipelines', pipelinesView: 'detail' })
  })

  it('routes a project-origin editor back to the project detail', () => {
    expect(editorBackTarget({ kind: 'project', name: 'orders' }))
      .toEqual({ section: 'projects', projectsView: 'detail' })
  })

  it('routes the default (list-origin) editor back to the pipeline list', () => {
    expect(editorBackTarget({ kind: 'list' }))
      .toEqual({ section: 'pipelines', pipelinesView: 'list' })
  })
})
