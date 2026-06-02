import { Suspense, lazy, useEffect, useState } from 'react'
import { createPipeline, listClusters, listProjects, renderPipelineYAML, updatePipeline } from '../api'
import { SecretRefsEditor } from './SecretRefsEditor'
import type { Pipeline, PipelineCluster, PipelineProject, SecretRef } from '../types'
import { roleOf, outputManaged, inputManaged } from '../projectRole'

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
  const [clusterRef, setClusterRef] = useState(editPipeline?.spec.clusterRef ?? '')
  const [clusters, setClusters] = useState<PipelineCluster[]>([])
  const [projectRef, setProjectRef] = useState(editPipeline?.spec.projectRef ?? '')
  const [projects, setProjects] = useState<PipelineProject[]>([])
  const [renderedYAML, setRenderedYAML] = useState<string>('')
  const [showRendered, setShowRendered] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    listClusters(namespace).then(setClusters).catch(() => setClusters([]))
    listProjects(namespace).then(setProjects).catch(() => setProjects([]))
  }, [namespace])

  const selectedProject = projects.find(p => p.metadata.name === projectRef)
  const role = projectRef ? roleOf(selectedProject?.spec.routes ?? [], name) : 'standalone'
  const managedKeys = [
    projectRef && outputManaged(role) ? 'output:' : '',
    projectRef && inputManaged(role) ? 'input:' : '',
  ].filter(Boolean)

  async function handleDeploy() {
    if (!name.trim()) {
      setError('Pipeline name must not be empty.')
      return
    }
    setSaving(true)
    setError(null)
    try {
      const spec = {
        rawYAML: text,
        ...(projectRef ? { projectRef } : (clusterRef ? { clusterRef } : {})),
        ...(secretRefs.length > 0 ? { secretRefs } : {}),
      }
      if (editPipeline) {
        await updatePipeline(namespace, name, spec, editPipeline.metadata.resourceVersion)
      } else {
        await createPipeline(namespace, name, spec)
      }
      onSaved()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Deploy failed'
      setError(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 16 }}>
        <button onClick={onBack} style={backBtnStyle}>← Back</button>
        <label style={{ fontSize: 14 }}>
          Pipeline name&nbsp;
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

      <div style={deploymentRowStyle}>
        <label style={{ fontSize: 14 }}>
          Run on&nbsp;
          <select value={clusterRef} disabled={!!projectRef}
                  onChange={e => { setClusterRef(e.target.value); if (e.target.value) setProjectRef('') }}
                  style={selectStyle}>
            <option value="">Own pod (default)</option>
            {clusters.map(c => (
              <option key={c.metadata.name} value={c.metadata.name}>{c.metadata.name}</option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 14 }}>
          Project&nbsp;
          <select value={projectRef} disabled={!!clusterRef}
                  onChange={e => { setProjectRef(e.target.value); if (e.target.value) setClusterRef('') }}
                  style={selectStyle}>
            <option value="">None</option>
            {projects.map(p => (
              <option key={p.metadata.name} value={p.metadata.name}>{p.metadata.name}</option>
            ))}
          </select>
        </label>
        {projectRef && <span style={roleBadgeStyle(role)}>role: {role}</span>}
        {clusters.length === 0 && projects.length === 0 && (
          <span style={{ fontSize: 12, color: '#9ca3af' }}>no clusters or projects in this namespace</span>
        )}
      </div>

      {managedKeys.length > 0 && (
        <div style={managedBannerStyle}>
          <strong>Project “{projectRef}” manages {managedKeys.join(' and ')}.</strong>{' '}
          Write only the non-managed keys; the operator injects the rest at deploy time.
        </div>
      )}

      <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
        <button onClick={() => setShowRendered(false)} disabled={!showRendered} style={tabBtnStyle(!showRendered)}>
          Your YAML
        </button>
        <button
          onClick={async () => {
            try {
              const yaml = await renderPipelineYAML(namespace, name || 'preview', {
                rawYAML: text,
                ...(projectRef ? { projectRef } : {}),
                ...(secretRefs.length ? { secretRefs } : {}),
              })
              setRenderedYAML(yaml)
              setShowRendered(true)
            } catch (e) {
              setError('Render failed: ' + (e as Error).message)
            }
          }}
          disabled={showRendered}
          style={tabBtnStyle(showRendered)}
        >
          Rendered (preview)
        </button>
      </div>

      <div style={{ border: '1px solid #d1d5db', borderRadius: 4, overflow: 'hidden', marginBottom: 12 }}>
        <Suspense fallback={<div style={{ padding: 16, color: '#888' }}>Loading editor…</div>}>
          <MonacoEditor
            height="500px"
            language="yaml"
            value={showRendered ? renderedYAML : text}
            onChange={v => { if (!showRendered) setText(v ?? '') }}
            options={{
              minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false,
              readOnly: showRendered,
            }}
          />
        </Suspense>
      </div>

      <SecretRefsEditor value={secretRefs} onChange={setSecretRefs} />

      {error && (
        <div style={errorBannerStyle}>{error}</div>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 8 }}>
        <button onClick={handleDeploy} disabled={saving} style={deployBtnStyle}>
          {saving ? 'Deploying…' : editPipeline ? 'Update' : 'Deploy'}
        </button>
      </div>
    </div>
  )
}

const deploymentRowStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12,
}
const selectStyle: React.CSSProperties = {
  padding: '4px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14, marginLeft: 4,
}
const inputStyle: React.CSSProperties = {
  padding: '5px 10px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14, marginLeft: 4,
}
const errorBannerStyle: React.CSSProperties = {
  background: '#fee2e2', color: '#dc2626', padding: '8px 12px',
  borderRadius: 4, fontSize: 13, marginBottom: 8,
}
const backBtnStyle: React.CSSProperties = {
  border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, color: '#3b82f6',
}
const deployBtnStyle: React.CSSProperties = {
  padding: '6px 20px', background: '#3b82f6', color: '#fff',
  border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 14,
}
const managedBannerStyle: React.CSSProperties = {
  border: '1px dashed #22c55e', background: '#f0fdf4', borderRadius: 6,
  padding: 12, fontSize: 12, color: '#166534', marginBottom: 12,
}
function roleBadgeStyle(role: string): React.CSSProperties {
  const map: Record<string, string> = {
    source: '#dbeafe', middle: '#ede9fe', sink: '#dcfce7', standalone: '#f3f4f6',
  }
  return {
    background: map[role] ?? '#f3f4f6', color: '#374151',
    padding: '2px 10px', borderRadius: 12, fontSize: 12, fontWeight: 600,
  }
}
function tabBtnStyle(active: boolean): React.CSSProperties {
  return {
    padding: '5px 12px', fontSize: 13, borderRadius: 6, cursor: 'pointer',
    border: '1px solid ' + (active ? '#1d4ed8' : '#d1d5db'),
    background: active ? '#eff6ff' : '#fff', color: active ? '#1d4ed8' : '#444',
  }
}
