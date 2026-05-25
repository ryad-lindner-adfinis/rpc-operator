import { Suspense, lazy, useEffect, useState } from 'react'
import { ComponentBox } from './ComponentBox'
import { SecretRefsEditor } from './SecretRefsEditor'
import { listClusters, renderPipelineYAML } from '../api'
import type { CatalogComponent, ComponentSpec, PipelineCluster, PipelineSpec } from '../types'

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

  const isRaw = !!spec.rawYAML
  const inCluster = !!spec.clusterRef

  useEffect(() => {
    listClusters(namespace).then(setClusters).catch(() => setClusters([]))
  }, [namespace])

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
      onChange({ ...spec, clusterRef: value })
    }
  }

  return (
    <div>
      {/* Deployment target */}
      <div style={deploymentRowStyle}>
        <label style={{ fontSize: 14 }}>
          Run on&nbsp;
          <select value={spec.clusterRef ?? ''} onChange={e => handleClusterChange(e.target.value)} style={selectStyle}>
            <option value="">Own pod (default)</option>
            {clusters.map(c => (
              <option key={c.metadata.name} value={c.metadata.name}>{c.metadata.name}</option>
            ))}
          </select>
        </label>
        {clusters.length === 0 && (
          <span style={{ fontSize: 12, color: '#9ca3af' }}>no clusters in this namespace</span>
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
          <ComponentBox
            title="Input"
            category="inputs"
            items={spec.input ? [spec.input] : []}
            catalogCache={catalogCache}
            onChange={setInput}
          />
          <ComponentBox
            title="Processors"
            category="processors"
            multi
            items={spec.processors ?? []}
            catalogCache={catalogCache}
            onChange={setProcessors}
          />
          <ComponentBox
            title="Output"
            category="outputs"
            items={spec.output ? [spec.output] : []}
            catalogCache={catalogCache}
            onChange={setOutput}
          />
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

      {inCluster ? (
        <div style={secretsDisabledStyle}>
          Secrets are not available for cluster-assigned pipelines. A stream shares the
          cluster's pod, where secrets are injected as pod-wide environment variables, so
          per-stream secret isolation is not possible (<code>SecretsUnsupportedInCluster</code>).
          {spec.secretRefs && spec.secretRefs.length > 0 && (
            <> Clear the {spec.secretRefs.length} existing secret(s) or switch back to "Own pod" before deploying.</>
          )}
        </div>
      ) : (
        <SecretRefsEditor
          value={spec.secretRefs ?? []}
          onChange={refs => onChange({ ...spec, secretRefs: refs })}
        />
      )}
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
const secretsDisabledStyle: React.CSSProperties = {
  border: '1px solid #fde68a', borderRadius: 6, padding: 12, marginTop: 12,
  background: '#fffbeb', color: '#92400e', fontSize: 13, lineHeight: 1.5,
}
