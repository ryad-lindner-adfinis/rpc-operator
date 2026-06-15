import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// Mock the lazy Monaco editor with a plain textarea so we can type YAML.
vi.mock('@monaco-editor/react', () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (v: string | undefined) => void }) => (
    <textarea aria-label="custom-config" value={value} onChange={e => onChange?.(e.target.value)} />
  ),
}))

import { CacheDrawer } from './CacheDrawer'

describe('CacheDrawer', () => {
  it('saves a managed natsKV cache with sizing fields', async () => {
    const onSave = vi.fn()
    render(<CacheDrawer existingNames={[]} onSave={onSave} onClose={() => {}} />)
    await userEvent.type(screen.getByLabelText(/Cache name/i), 'shared')
    await userEvent.type(screen.getByLabelText(/TTL/i), '1h')
    await userEvent.type(screen.getByLabelText(/History/i), '3')
    await userEvent.click(screen.getByRole('button', { name: /Save cache/i }))
    expect(onSave).toHaveBeenCalledWith({ name: 'shared', natsKV: { ttl: '1h', history: 3 } })
  })

  it('rejects a duplicate name in new mode', async () => {
    const onSave = vi.fn()
    render(<CacheDrawer existingNames={['shared']} onSave={onSave} onClose={() => {}} />)
    await userEvent.type(screen.getByLabelText(/Cache name/i), 'shared')
    await userEvent.click(screen.getByRole('button', { name: /Save cache/i }))
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText(/already exists/i)).toBeInTheDocument()
  })

  it('saves a custom config parsed from YAML', async () => {
    const onSave = vi.fn()
    render(<CacheDrawer existingNames={[]} onSave={onSave} onClose={() => {}} />)
    await userEvent.type(screen.getByLabelText(/Cache name/i), 'redis-side')
    await userEvent.click(screen.getByRole('radio', { name: /Custom/i }))
    // fireEvent.change sets the value verbatim (userEvent.type would treat `{`/newlines specially).
    fireEvent.change(screen.getByLabelText('custom-config'), { target: { value: 'redis:\n  url: redis://r:6379' } })
    await userEvent.click(screen.getByRole('button', { name: /Save cache/i }))
    expect(onSave).toHaveBeenCalledWith({ name: 'redis-side', config: { redis: { url: 'redis://r:6379' } } })
  })

  it('pre-fills and locks the name when editing', () => {
    render(<CacheDrawer cache={{ name: 'shared', natsKV: { history: 2 } }}
      existingNames={['shared']} onSave={() => {}} onClose={() => {}} />)
    const name = screen.getByLabelText(/Cache name/i) as HTMLInputElement
    expect(name).toHaveValue('shared')
    expect(name).toHaveAttribute('readonly')
    expect(screen.getByLabelText(/History/i)).toHaveValue(2)
  })

  it('does not wrap the custom editor in a <label> (focus-stealing regression)', async () => {
    // A <label> re-dispatches clicks to Monaco's inner <textarea>, stealing focus
    // so the editor can't be typed in. The editor must have no <label> ancestor.
    render(<CacheDrawer existingNames={[]} onSave={() => {}} onClose={() => {}} />)
    await userEvent.click(screen.getByRole('radio', { name: /Custom/i }))
    expect(screen.getByLabelText('custom-config').closest('label')).toBeNull()
  })

  it('rejects a custom config that is a YAML array', async () => {
    const onSave = vi.fn()
    render(<CacheDrawer existingNames={[]} onSave={onSave} onClose={() => {}} />)
    await userEvent.type(screen.getByLabelText(/Cache name/i), 'bad')
    await userEvent.click(screen.getByRole('radio', { name: /Custom/i }))
    fireEvent.change(screen.getByLabelText('custom-config'), { target: { value: '- redis\n- stdout' } })
    await userEvent.click(screen.getByRole('button', { name: /Save cache/i }))
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByText(/YAML object/i)).toBeInTheDocument()
  })
})
