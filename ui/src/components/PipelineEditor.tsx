import { Suspense, lazy, useEffect, useState } from 'react'
import { ComponentBox } from './ComponentBox'
import { SecretRefsEditor } from './SecretRefsEditor'
import { listClusters, listProjects, renderPipelineYAML } from '../api'
import type { CatalogComponent, ComponentSpec, PipelineCluster, PipelineProject, PipelineSpec } from '../types'
import { roleOf, outputManaged, inputManaged } from '../projectRole'

const MonacoEditor = lazy(() =>
  import('@monaco-editor/react').then(m => ({ default: m.default })),
)

interface Props {
  namespace: string
  name: string
  spec: PipelineSpec
  catalogCache: Map<string, CatalogComponent>
  onChange: (spec: PipelineSpec) => void
}

export function PipelineEditor({ namespace, name, spec, catalogCache, onChange }: Props) {
  const [mode, setMode] = useState<'visual' | 'yaml'>('visual')
  const [yamlText, setYamlText] = useState('')
  const [yamlLoading, setYamlLoading] = useState(false)
  const [yamlError, setYamlError] = useState<string>()
  const [clusters, setClusters] = useState<PipelineCluster[]>([])
  const [projects, setProjects] = useState<PipelineProject[]>([])

  const isRaw = !!spec.rawYAML

  useEffect(() => {
    listClusters(namespace).then(setClusters).catch(() => setClusters([]))
    listProjects(namespace).then(setProjects).catch(() => setProjects([]))
  }, [namespace])

  const selectedProject = projects.find(p => p.metadata.name === spec.projectRef)
  const role = spec.projectRef ? roleOf(selectedProject?.spec.routes ?? [], name) : 'standalone'
  const outManaged = !!spec.projectRef && outputManaged(role)
  const inManaged = !!spec.projectRef && inputManaged(role)

  async function switchToYaml() {
    if (!isRaw && (!spec.input || !spec.output)) {
      setYamlError('Input and Output must be configured before switching to YAML mode.')
      return
    }
    setYamlError(undefined)
    setYamlLoading(true)
    try {
      const text = isRaw
        ? (spec.rawYAML ?? '')
        : await renderPipelineYAML(namespace, name || 'preview', spec)
      setYamlText(text)
      setMode('yaml')
    } catch (e) {
      setYamlError('Render failed: ' + (e as Error).message)
    } finally {
      setYamlLoading(false)
    }
  }

  function switchToVisual() {
    setMode('visual')
  }

  function handleYamlChange(text: string | undefined) {
    const t = text ?? ''
    setYamlText(t)
    onChange({
      rawYAML: t,
      ...(spec.clusterRef ? { clusterRef: spec.clusterRef } : {}),
      ...(spec.secretRefs && spec.secretRefs.length > 0 ? { secretRefs: spec.secretRefs } : {}),
    })
  }

  function setInput(items: ComponentSpec[]) {
    onChange({ ...spec, input: items[0] })
  }
  function setProcessors(items: ComponentSpec[]) {
    onChange({ ...spec, processors: items })
  }
  function setOutput(items: ComponentSpec[]) {
    onChange({ ...spec, output: items[0] })
  }

  function handleClusterChange(value: string) {
    if (value === '') {
      // "Own pod" — drop clusterRef.
      const { clusterRef: _omit, ...rest } = spec
      onChange(rest)
    } else {
      // Mutually exclusive with projectRef — drop it.
      const { projectRef: _drop, ...rest } = spec
      onChange({ ...rest, clusterRef: value })
    }
  }

  function handleProjectChange(value: string) {
    if (value === '') {
      const { projectRef: _omit, ...rest } = spec
      onChange(rest)
    } else {
      // Mutually exclusive with clusterRef — drop it.
      const { clusterRef: _drop, ...rest } = spec
      onChange({ ...rest, projectRef: value })
    }
  }

  return (
    <div>
      {/* Deployment target */}
      <div style={deploymentRowStyle}>
        <label style={{ fontSize: 14 }}>
          Run on&nbsp;
          <select value={spec.clusterRef ?? ''} disabled={!!spec.projectRef}
                  onChange={e => handleClusterChange(e.target.value)} style={selectStyle}>
            <option value="">Own pod (default)</option>
            {clusters.map(c => (
              <option key={c.metadata.name} value={c.metadata.name}>{c.metadata.name}</option>
            ))}
          </select>
        </label>
        <label style={{ fontSize: 14 }}>
          Project&nbsp;
          <select value={spec.projectRef ?? ''} disabled={!!spec.clusterRef}
                  onChange={e => handleProjectChange(e.target.value)} style={selectStyle}>
            <option value="">None</option>
            {projects.map(p => (
              <option key={p.metadata.name} value={p.metadata.name}>{p.metadata.name}</option>
            ))}
          </select>
        </label>
        {spec.projectRef && (
          <span style={roleBadgeStyle(role)}>role: {role}</span>
        )}
        {clusters.length === 0 && projects.length === 0 && (
          <span style={{ fontSize: 12, color: '#9ca3af' }}>no clusters or projects in this namespace</span>
        )}
      </div>

      <div style={{ marginBottom: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
        <button onClick={switchToVisual} disabled={mode === 'visual'}>
          Visual
        </button>
        <button onClick={switchToYaml} disabled={mode === 'yaml' || yamlLoading}>
          {yamlLoading ? 'Loading YAML…' : 'YAML'}
        </button>
        {isRaw && (
          <span style={rawBadgeStyle} title="Pipeline was edited in YAML mode and will be deployed as RAW YAML.">
            RAW YAML
          </span>
        )}
        {yamlError && <span style={{ color: '#dc2626', fontSize: 13 }}>{yamlError}</span>}
      </div>

      {mode === 'visual' && !isRaw && (
        <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
          {inManaged ? (
            <ManagedSection side="Input" role={role} project={spec.projectRef!} />
          ) : (
            <ComponentBox
              title="Input"
              category="inputs"
              items={spec.input ? [spec.input] : []}
              catalogCache={catalogCache}
              onChange={setInput}
            />
          )}
          <ComponentBox
            title="Processors"
            category="processors"
            multi
            items={spec.processors ?? []}
            catalogCache={catalogCache}
            onChange={setProcessors}
          />
          {outManaged ? (
            <ManagedSection side="Output" role={role} project={spec.projectRef!} />
          ) : (
            <ComponentBox
              title="Output"
              category="outputs"
              items={spec.output ? [spec.output] : []}
              catalogCache={catalogCache}
              onChange={setOutput}
            />
          )}
        </div>
      )}

      {mode === 'visual' && isRaw && (
        <div style={rawNoticeStyle}>
          This pipeline is in RAW YAML mode. Structured editing is not available — switch to the YAML tab to edit the configuration.
        </div>
      )}

      {mode === 'yaml' && (
        <div>
          {isRaw && (
            <div style={rawBannerStyle}>
              YAML editing active: on deploy the pipeline will be saved as <code>spec.rawYAML</code>. Structured editing will only be possible by creating a new pipeline.
            </div>
          )}
          <Suspense fallback={<div>Loading editor…</div>}>
            <MonacoEditor
              height="400px"
              language="yaml"
              value={yamlText}
              onChange={handleYamlChange}
              options={{ minimap: { enabled: false }, wordWrap: 'on', fontSize: 13 }}
            />
          </Suspense>
        </div>
      )}

      <SecretRefsEditor
        value={spec.secretRefs ?? []}
        onChange={refs => onChange({ ...spec, secretRefs: refs })}
      />
    </div>
  )
}

const rawBadgeStyle: React.CSSProperties = {
  fontSize: 11, fontWeight: 600, color: '#fff', background: '#3b82f6',
  padding: '2px 8px', borderRadius: 10, letterSpacing: 0.3,
}
const rawBannerStyle: React.CSSProperties = {
  background: '#fef3c7', color: '#92400e', padding: '8px 12px',
  borderRadius: 4, fontSize: 13, marginBottom: 8, border: '1px solid #fde68a',
}
const rawNoticeStyle: React.CSSProperties = {
  background: '#eff6ff', color: '#1e40af', padding: '12px 16px',
  borderRadius: 4, fontSize: 14, border: '1px solid #bfdbfe',
}
const deploymentRowStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12,
}
const selectStyle: React.CSSProperties = {
  padding: '4px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14, marginLeft: 4,
}

function ManagedSection({ side, role, project }: { side: 'Input' | 'Output'; role: string; project: string }) {
  return (
    <div style={{ flex: 1, minWidth: 0 }}>
      <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 6 }}>{side}</div>
      <div style={managedBannerStyle}>
        <strong>Managed by project “{project}”.</strong>
        <div style={{ marginTop: 4, color: '#15803d' }}>
          The operator injects this {side.toLowerCase()} ({role} pipeline) as a <code>nats_jetstream</code> block at deploy time.
          Use the project’s tactical map to change routing.
        </div>
      </div>
    </div>
  )
}
const managedBannerStyle: React.CSSProperties = {
  border: '1px dashed #22c55e', background: '#f0fdf4', borderRadius: 6,
  padding: 12, fontSize: 12, color: '#166534',
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
