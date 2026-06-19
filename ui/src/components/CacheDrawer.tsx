import { lazy, Suspense, useState } from 'react'
import yaml from 'js-yaml'
import type { ProjectCacheResource } from '../types'

const MonacoEditor = lazy(() => import('@monaco-editor/react').then(m => ({ default: m.default })))

interface Props {
  /** Editing an existing cache → prefilled, name read-only. */
  cache?: ProjectCacheResource
  /** New cache seeded with this name (e.g. adding an undeclared reference). */
  prefillName?: string
  /** Existing cache names in the draft, for duplicate detection in new mode. */
  existingNames: string[]
  onSave: (cache: ProjectCacheResource) => void
  onClose: () => void
}

type Kind = 'managed' | 'custom'

export function CacheDrawer({ cache, prefillName, existingNames, onSave, onClose }: Props) {
  const editing = !!cache
  const [name, setName] = useState(cache?.name ?? prefillName ?? '')
  const [kind, setKind] = useState<Kind>(cache?.config != null ? 'custom' : 'managed')
  const [ttl, setTtl] = useState(cache?.natsKV?.ttl ?? '')
  const [history, setHistory] = useState(cache?.natsKV?.history != null ? String(cache.natsKV.history) : '')
  const [maxBytes, setMaxBytes] = useState(cache?.natsKV?.maxBytes ?? '')
  const [customText, setCustomText] = useState(
    cache?.config != null ? yaml.dump(cache.config) : '',
  )
  const [error, setError] = useState<string>()

  function handleSave() {
    setError(undefined)
    const n = name.trim()
    if (!n) { setError('Cache name is required.'); return }
    if (!/^[a-z]([-a-z0-9]*[a-z0-9])?$/.test(n)) {
      setError('Cache name must be a DNS-1123 label (lower-case, digits, dashes).'); return
    }
    if (n.length > 63) { setError('Cache name must be at most 63 characters.'); return }
    if (!editing && existingNames.includes(n)) { setError(`A cache named "${n}" already exists.`); return }

    if (kind === 'custom') {
      let config: unknown
      try {
        config = yaml.load(customText)
      } catch {
        setError('Custom config is not valid YAML.'); return
      }
      if (config == null || typeof config !== 'object' || Array.isArray(config)) {
        setError('Custom config must be a YAML object (e.g. redis: { url: … }).'); return
      }
      onSave({ name: n, config })
      return
    }

    const natsKV: NonNullable<ProjectCacheResource['natsKV']> = {}
    if (ttl.trim()) natsKV.ttl = ttl.trim()
    if (history.trim()) {
      const h = Number(history)
      if (!Number.isInteger(h) || h < 1 || h > 64) { setError('History must be an integer between 1 and 64.'); return }
      natsKV.history = h
    }
    if (maxBytes.trim()) natsKV.maxBytes = maxBytes.trim()
    onSave({ name: n, natsKV })
  }

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div style={drawerStyle} onClick={e => e.stopPropagation()}>
        <h3 style={{ margin: '0 0 16px', fontSize: 16 }}>{editing ? 'Edit cache' : 'New cache'}</h3>

        <label style={labelStyle}>
          Cache name
          <input value={name} onChange={e => setName(e.target.value)}
                 readOnly={editing} style={inputStyle} placeholder="shared-state" />
        </label>

        <div style={{ marginTop: 12, fontSize: 13, fontWeight: 600 }}>Type</div>
        <div role="radiogroup" style={{ display: 'flex', gap: 16, marginTop: 6 }}>
          <label style={radioStyle}>
            <input type="radio" name="cache-kind" aria-label="Managed (NATS KV)"
                   checked={kind === 'managed'} onChange={() => setKind('managed')} />
            Managed (NATS KV)
          </label>
          <label style={radioStyle}>
            <input type="radio" name="cache-kind" aria-label="Custom config"
                   checked={kind === 'custom'} onChange={() => setKind('custom')} />
            Custom config
          </label>
        </div>

        {kind === 'managed' ? (
          <>
            <label style={labelStyle}>
              TTL (optional, e.g. 1h)
              <input value={ttl} onChange={e => setTtl(e.target.value)} style={inputStyle} placeholder="1h" />
            </label>
            <label style={labelStyle}>
              History (optional, 1–64)
              <input type="number" value={history} onChange={e => setHistory(e.target.value)} style={inputStyle} placeholder="1" />
            </label>
            <label style={labelStyle}>
              MaxBytes (optional, e.g. 100Mi)
              <input value={maxBytes} onChange={e => setMaxBytes(e.target.value)} style={inputStyle} placeholder="100Mi" />
            </label>
          </>
        ) : (
          // NB: must NOT be a <label> — a label re-dispatches clicks to Monaco's
          // inner <textarea>, stealing focus so the editor can't be typed in.
          <div style={{ ...labelStyle, marginTop: 12 }}>
            <span>Custom config (YAML cache block)</span>
            <div style={{ border: '1px solid #ccc', borderRadius: 4, overflow: 'hidden', marginTop: 4 }}>
              <Suspense fallback={<div style={{ padding: 12, color: '#888' }}>Loading editor…</div>}>
                <MonacoEditor
                  height="220px"
                  language="yaml"
                  value={customText}
                  onChange={v => setCustomText(v ?? '')}
                  options={{ minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false }}
                />
              </Suspense>
            </div>
          </div>
        )}

        {error && <div style={{ color: '#dc2626', fontSize: 13, marginTop: 12 }}>{error}</div>}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 20 }}>
          <button onClick={onClose} style={cancelBtnStyle}>Cancel</button>
          <button onClick={handleSave} style={saveBtnStyle}>Save cache</button>
        </div>
      </div>
    </div>
  )
}

const overlayStyle: React.CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.25)', zIndex: 50,
  display: 'flex', justifyContent: 'flex-end',
}
const drawerStyle: React.CSSProperties = {
  width: 420, maxWidth: '90vw', height: '100%', background: '#fff',
  padding: 24, overflowY: 'auto', boxShadow: '-4px 0 12px rgba(0,0,0,0.1)',
}
const labelStyle: React.CSSProperties = {
  display: 'flex', flexDirection: 'column', gap: 4, fontSize: 13, color: '#444', marginTop: 10,
}
const radioStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, color: '#444',
}
const inputStyle: React.CSSProperties = {
  padding: '6px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 13,
}
const saveBtnStyle: React.CSSProperties = {
  padding: '7px 14px', fontSize: 13, background: '#1d4ed8', color: '#fff',
  border: 'none', borderRadius: 6, cursor: 'pointer',
}
const cancelBtnStyle: React.CSSProperties = {
  padding: '7px 14px', fontSize: 13, background: '#fff', color: '#444',
  border: '1px solid #ccc', borderRadius: 6, cursor: 'pointer',
}
