import type { ProjectRoute } from './types'

export type PipelineRole = 'standalone' | 'source' | 'middle' | 'sink'

/** Mirrors internal/projectroute.RoleOf — pure derivation from the route table. */
export function roleOf(routes: ProjectRoute[], pipeline: string): PipelineRole {
  const producer = routes.some(r => r.from === pipeline)
  const consumer = routes.some(r => r.to.some(t => t.pipeline === pipeline))
  if (producer && consumer) return 'middle'
  if (producer) return 'source'
  if (consumer) return 'sink'
  return 'standalone'
}

/** True when the operator manages this pipeline's output (producer side). */
export function outputManaged(role: PipelineRole): boolean {
  return role === 'source' || role === 'middle'
}

/** True when the operator manages this pipeline's input (consumer side). */
export function inputManaged(role: PipelineRole): boolean {
  return role === 'sink' || role === 'middle'
}
