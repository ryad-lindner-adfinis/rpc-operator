// Where a pipeline detail view was opened from. Used to route its Back button
// to the originating view instead of always dumping the user on the pipeline
// list. One level deep by design (see F50.4 spec, "Scope limit").
export type PipelineOrigin =
  | { kind: 'pipelines' }              // opened from the pipeline list (default)
  | { kind: 'project'; name: string }  // opened from a ProjectDetail
  | { kind: 'cluster'; name: string }  // opened from a ClusterDetail

export type BackTarget =
  | { section: 'pipelines'; pipelinesView: 'list' }
  | { section: 'projects'; projectsView: 'detail' }
  | { section: 'clusters'; clustersView: 'detail' }

export function pipelineBackTarget(origin: PipelineOrigin): BackTarget {
  switch (origin.kind) {
    case 'project':  return { section: 'projects', projectsView: 'detail' }
    case 'cluster':  return { section: 'clusters', clustersView: 'detail' }
    case 'pipelines': return { section: 'pipelines', pipelinesView: 'list' }
    default: {
      // Exhaustiveness guard: a new PipelineOrigin.kind must be handled above,
      // not silently fall through to the pipeline list.
      const _exhaustive: never = origin
      return _exhaustive
    }
  }
}

// Where the pipeline EDITOR (visual or raw) was opened from. Distinct from
// PipelineOrigin (which tracks the DETAIL's origin): editing from the list has
// no detail. Used to route the editor's Back/Save. One level deep by design.
export type EditorOrigin =
  | { kind: 'list' }                   // Edit/New from the pipeline list (default)
  | { kind: 'detail' }                 // Edit from a pipeline detail
  | { kind: 'project'; name: string }  // New pipeline from a project

export type EditorBackTarget =
  | { section: 'pipelines'; view: 'list' | 'detail' }
  | { section: 'projects'; projectsView: 'detail' }

export function editorBackTarget(origin: EditorOrigin): EditorBackTarget {
  switch (origin.kind) {
    case 'detail':  return { section: 'pipelines', view: 'detail' }
    case 'project': return { section: 'projects', projectsView: 'detail' }
    case 'list':    return { section: 'pipelines', view: 'list' }
    default: {
      // Exhaustiveness guard: a new EditorOrigin.kind must be handled above.
      const _exhaustive: never = origin
      return _exhaustive
    }
  }
}
