import { useState } from 'react'
import type { ProjectRoute, ProjectRouteTarget } from '../types'

interface Props {
  pipelines: string[]
  /** When set, the drawer edits this route; otherwise it creates a new one. */
  route?: ProjectRoute
  onSave: (route: ProjectRoute) => void
  onClose: () => void
}

export function RouterDrawer({ pipelines, route, onSave, onClose }: Props) {
  const [name, setName] = useState(route?.name ?? '')
  const [from, setFrom] = useState(route?.from ?? '')
  const [targets, setTargets] = useState<ProjectRouteTarget[]>(
    route?.to?.length ? route.to : [{ pipeline: '' }],
  )
  const [error, setError] = useState<string>()

  function setTarget(i: number, patch: Partial<ProjectRouteTarget>) {
    setTargets(ts => ts.map((t, idx) => (idx === i ? { ...t, ...patch } : t)))
  }
  function addTarget() { setTargets(ts => [...ts, { pipeline: '' }]) }
  function removeTarget(i: number) { setTargets(ts => ts.filter((_, idx) => idx !== i)) }

  function handleSave() {
    if (!name.trim()) { setError('Route name is required.'); return }
    if (!/^[a-z]([-a-z0-9]*[a-z0-9])?$/.test(name)) {
      setError('Route name must be a DNS-1123 label (lower-case, digits, dashes).'); return
    }
    if (!from) { setError('From pipeline is required.'); return }
    const cleaned = targets.filter(t => t.pipeline)
    if (cleaned.length === 0) { setError('At least one target is required.'); return }
    onSave({
      name: name.trim(),
      from,
      to: cleaned.map(t => (t.when?.trim() ? { pipeline: t.pipeline, when: t.when.trim() } : { pipeline: t.pipeline })),
    })
  }

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div style={drawerStyle} onClick={e => e.stopPropagation()}>
        <h3 style={{ margin: '0 0 16px', fontSize: 16 }}>{route ? 'Edit router' : 'New router'}</h3>

        <label style={labelStyle}>
          Route name
          <input value={name} onChange={e => setName(e.target.value)}
                 readOnly={!!route} style={inputStyle} placeholder="ingest-fan" />
        </label>

        <label style={labelStyle}>
          From
          <select value={from} onChange={e => setFrom(e.target.value)} style={inputStyle}>
            <option value="">— select source pipeline —</option>
            {pipelines.map(p => <option key={p} value={p}>{p}</option>)}
          </select>
        </label>

        <div style={{ marginTop: 12, fontSize: 13, fontWeight: 600 }}>Targets</div>
        {targets.map((t, i) => (
          <div key={i} style={{ border: '1px solid #eee', borderRadius: 6, padding: 8, marginTop: 8 }}>
            <label style={labelStyle}>
              {`Target ${i + 1}`}
              <select value={t.pipeline} onChange={e => setTarget(i, { pipeline: e.target.value })} style={inputStyle}>
                <option value="">— select target pipeline —</option>
                {pipelines.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
            </label>
            <label style={labelStyle}>
              {`When ${i + 1} (optional Bloblang predicate)`}
              <input value={t.when ?? ''} onChange={e => setTarget(i, { when: e.target.value })}
                     style={inputStyle} placeholder='this.level == "high"' />
            </label>
            {targets.length > 1 && (
              <button onClick={() => removeTarget(i)} style={removeBtnStyle}>Remove target</button>
            )}
          </div>
        ))}
        <button onClick={addTarget} style={addBtnStyle}>+ Add target</button>

        {error && <div style={{ color: '#dc2626', fontSize: 13, marginTop: 12 }}>{error}</div>}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 20 }}>
          <button onClick={onClose} style={cancelBtnStyle}>Cancel</button>
          <button onClick={handleSave} style={saveBtnStyle}>Save router</button>
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
const addBtnStyle: React.CSSProperties = {
  marginTop: 8, padding: '5px 10px', fontSize: 12, background: '#f3f4f6',
  border: '1px solid #d1d5db', borderRadius: 4, cursor: 'pointer',
}
const removeBtnStyle: React.CSSProperties = {
  marginTop: 6, padding: '3px 8px', fontSize: 11, background: '#fff',
  border: '1px solid #fca5a5', color: '#dc2626', borderRadius: 4, cursor: 'pointer',
}
