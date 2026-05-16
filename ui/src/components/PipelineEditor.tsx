import { Suspense, lazy, useState } from 'react'
import { ComponentBox } from './ComponentBox'
import { SecretRefsEditor } from './SecretRefsEditor'
import { renderPipelineYAML } from '../api'
import type { CatalogComponent, ComponentSpec, PipelineSpec } from '../types'

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

  const isRaw = !!spec.rawYAML

  async function switchToYaml() {
    if (!isRaw && (!spec.input || !spec.output)) {
      setYamlError('Input und Output müssen belegt sein bevor in den YAML-Modus gewechselt wird.')
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
      setYamlError('Render fehlgeschlagen: ' + (e as Error).message)
    } finally {
      setYamlLoading(false)
    }
  }

  function switchToVisual() {
    if (isRaw) {
      // Pipeline already in raw mode — visual tab will show a notice.
      setMode('visual')
      return
    }
    setMode('visual')
  }

  function handleYamlChange(text: string | undefined) {
    const t = text ?? ''
    setYamlText(t)
    // Editing the rendered YAML promotes the pipeline to raw mode so DeployBar
    // sends spec.rawYAML instead of the structured fields.
    onChange({
      rawYAML: t,
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

  return (
    <div>
      <div style={{ marginBottom: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
        <button onClick={switchToVisual} disabled={mode === 'visual'}>
          Visuell
        </button>
        <button onClick={switchToYaml} disabled={mode === 'yaml' || yamlLoading}>
          {yamlLoading ? 'Lade YAML…' : 'YAML'}
        </button>
        {isRaw && (
          <span style={rawBadgeStyle} title="Pipeline wurde im YAML-Modus editiert und wird als RAW YAML deployed.">
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
          Diese Pipeline ist im RAW-YAML-Modus. Strukturierte Bearbeitung ist nicht
          möglich — wechsle in den YAML-Tab, um die Konfiguration zu editieren.
        </div>
      )}

      {mode === 'yaml' && (
        <div>
          {isRaw && (
            <div style={rawBannerStyle}>
              YAML-Edit aktiv: Beim Deploy wird die Pipeline als <code>spec.rawYAML</code> gespeichert.
              Strukturierte Bearbeitung ist danach nur über erneutes Anlegen möglich.
            </div>
          )}
          <Suspense fallback={<div>Lade Editor…</div>}>
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
