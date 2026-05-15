import { Suspense, lazy, useState } from 'react'
import { createPipeline, updatePipeline } from '../api'
import { SecretRefsEditor } from './SecretRefsEditor'
import type { Pipeline, SecretRef } from '../types'

const MonacoEditor = lazy(() =>
  import('@monaco-editor/react').then(m => ({ default: m.default })),
)

interface Props {
  namespace: string
  editPipeline?: Pipeline
  onBack: () => void
  onSaved: () => void
}

export function RawPipelineEditor({ namespace, editPipeline, onBack, onSaved }: Props) {
  const [name, setName] = useState(editPipeline?.metadata.name ?? '')
  const [text, setText] = useState(editPipeline?.spec.rawYAML ?? '')
  const [secretRefs, setSecretRefs] = useState<SecretRef[]>(editPipeline?.spec.secretRefs ?? [])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  async function handleDeploy() {
    if (!name.trim()) {
      setError('Pipeline-Name darf nicht leer sein.')
      return
    }
    setSaving(true)
    setError(null)
    try {
      const spec = { rawYAML: text, ...(secretRefs.length > 0 ? { secretRefs } : {}) }
      if (editPipeline) {
        await updatePipeline(namespace, name, spec, editPipeline.metadata.resourceVersion)
      } else {
        await createPipeline(namespace, name, spec)
      }
      onSaved()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Deploy fehlgeschlagen'
      setError(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 16 }}>
        <label style={{ fontSize: 14 }}>
          Pipeline-Name&nbsp;
          <input
            value={name}
            onChange={e => setName(e.target.value)}
            readOnly={!!editPipeline}
            style={{
              ...inputStyle,
              background: editPipeline ? '#f5f5f5' : undefined,
              color: editPipeline ? '#888' : undefined,
            }}
          />
        </label>
        <span style={{ fontSize: 13, color: '#888' }}>Namespace: {namespace}</span>
        <span style={{ fontSize: 12, color: '#3b82f6', marginLeft: 'auto' }}>RAW YAML Mode</span>
      </div>

      <div style={{ border: '1px solid #d1d5db', borderRadius: 4, overflow: 'hidden', marginBottom: 12 }}>
        <Suspense fallback={<div style={{ padding: 16, color: '#888' }}>Editor lädt…</div>}>
          <MonacoEditor
            height="500px"
            language="yaml"
            value={text}
            onChange={v => setText(v ?? '')}
            options={{ minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false }}
          />
        </Suspense>
      </div>

      <SecretRefsEditor value={secretRefs} onChange={setSecretRefs} />

      {error && (
        <div style={errorBannerStyle}>{error}</div>
      )}

      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8 }}>
        <button onClick={onBack} style={backBtnStyle}>← Zurück</button>
        <button onClick={handleDeploy} disabled={saving} style={deployBtnStyle}>
          {saving ? 'Deploying…' : editPipeline ? 'Update' : 'Deploy'}
        </button>
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  padding: '5px 10px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14, marginLeft: 4,
}
const errorBannerStyle: React.CSSProperties = {
  background: '#fee2e2', color: '#dc2626', padding: '8px 12px',
  borderRadius: 4, fontSize: 13, marginBottom: 8,
}
const backBtnStyle: React.CSSProperties = {
  padding: '6px 16px', background: 'none', border: '1px solid #ccc',
  borderRadius: 4, cursor: 'pointer', fontSize: 14,
}
const deployBtnStyle: React.CSSProperties = {
  padding: '6px 20px', background: '#3b82f6', color: '#fff',
  border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 14,
}
