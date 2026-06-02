import { describe, it, expect } from 'vitest'
import { roleOf } from './projectRole'
import type { ProjectRoute } from './types'

const routes: ProjectRoute[] = [
  { name: 'a', from: 'src', to: [{ pipeline: 'mid' }] },
  { name: 'b', from: 'mid', to: [{ pipeline: 'sink' }] },
]

describe('roleOf', () => {
  it('classifies source / middle / sink / standalone', () => {
    expect(roleOf(routes, 'src')).toBe('source')
    expect(roleOf(routes, 'mid')).toBe('middle')
    expect(roleOf(routes, 'sink')).toBe('sink')
    expect(roleOf(routes, 'lonely')).toBe('standalone')
  })
})
