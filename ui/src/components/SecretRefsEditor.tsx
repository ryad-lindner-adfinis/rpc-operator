import { X } from 'lucide-react'
import type { SecretRef } from '../types'

interface Props {
  value: SecretRef[]
  onChange: (refs: SecretRef[]) => void
}

const emptyRef = (): SecretRef => ({ envVar: '', secretName: '', key: '' })

export function SecretRefsEditor({ value, onChange }: Props) {
  function update(idx: number, field: keyof SecretRef, val: string) {
    onChange(value.map((r, i) => i === idx ? { ...r, [field]: val } : r))
  }
  function remove(idx: number) {
    onChange(value.filter((_, i) => i !== idx))
  }
  function add() {
    onChange([...value, emptyRef()])
  }

  return (
    <div style={boxStyle}>
      <div style={headerStyle}>Secrets (Umgebungsvariablen)</div>
      {value.length > 0 && (
        <div style={gridHeaderStyle}>
          <span>ENV_VAR</span><span>Secret-Name</span><span>Key</span><span />
        </div>
      )}
      {value.map((r, i) => (
        <div key={i} style={rowStyle}>
          <input
            placeholder="MY_SECRET"
            value={r.envVar}
            onChange={e => update(i, 'envVar', e.target.value)}
            style={inputStyle}
          />
          <input
            placeholder="my-k8s-secret"
            value={r.secretName}
            onChange={e => update(i, 'secretName', e.target.value)}
            style={inputStyle}
          />
          <input
            placeholder="password"
            value={r.key}
            onChange={e => update(i, 'key', e.target.value)}
            style={inputStyle}
          />
          <button onClick={() => remove(i)} style={removeBtnStyle}><X size={14} /></button>
        </div>
      ))}
      <button onClick={add} style={addBtnStyle}>+ Secret hinzufügen</button>
      <p style={hintStyle}>Im YAML referenzieren als <code>{'${MY_SECRET}'}</code></p>
    </div>
  )
}

const boxStyle: React.CSSProperties = {
  border: '1px solid #dde', borderRadius: 6, padding: 12, marginTop: 12, background: '#fafafa',
}
const headerStyle: React.CSSProperties = {
  fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#334',
}
const gridHeaderStyle: React.CSSProperties = {
  display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 28px', gap: 4,
  fontSize: 11, color: '#888', marginBottom: 4,
}
const rowStyle: React.CSSProperties = {
  display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 28px', gap: 4, marginBottom: 4,
}
const inputStyle: React.CSSProperties = {
  padding: '4px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 13,
}
const removeBtnStyle: React.CSSProperties = {
  color: '#c00', border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, padding: 0,
}
const addBtnStyle: React.CSSProperties = {
  marginTop: 4, padding: '5px 12px', cursor: 'pointer', borderRadius: 4,
  border: '1px dashed #aab', background: 'none', fontSize: 13, width: '100%',
}
const hintStyle: React.CSSProperties = {
  margin: '6px 0 0', fontSize: 11, color: '#888',
}
