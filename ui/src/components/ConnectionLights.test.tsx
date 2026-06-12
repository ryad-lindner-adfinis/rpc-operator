import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConnectionLights } from './ConnectionLights'

describe('ConnectionLights', () => {
  it('shows "connected" for up/up', () => {
    render(<ConnectionLights state={{ input: 'up', output: 'up' }} />)
    expect(screen.getByText('Input: connected')).toBeInTheDocument()
    expect(screen.getByText('Output: connected')).toBeInTheDocument()
  })

  it('shows "disconnected" for down', () => {
    render(<ConnectionLights state={{ input: 'down', output: 'up' }} />)
    expect(screen.getByText('Input: disconnected')).toBeInTheDocument()
    expect(screen.getByText('Output: connected')).toBeInTheDocument()
  })

  it('shows "unknown" for unknown state', () => {
    render(<ConnectionLights state={{ input: 'unknown', output: 'unknown' }} />)
    expect(screen.getByText('Input: unknown')).toBeInTheDocument()
    expect(screen.getByText('Output: unknown')).toBeInTheDocument()
  })

  it('shows "unknown" when state is undefined', () => {
    render(<ConnectionLights state={undefined} />)
    expect(screen.getByText('Input: unknown')).toBeInTheDocument()
    expect(screen.getByText('Output: unknown')).toBeInTheDocument()
  })
})
