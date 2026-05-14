import { Suspense, lazy, useState } from 'react'
import { ComponentBox } from './ComponentBox'
import { specToYaml, yamlToSpec } from '../yaml-codec'
import type { CatalogComponent, ComponentSpec, PipelineSpec } from '../types'

const MonacoEditor = lazy(() =>
  import('@monaco-editor/react').then(m => ({ default: m.default })),
)

interface Props {
  spec: PipelineSpec
  catalogCache: Map<string, CatalogComponent>
  onChange: (spec: PipelineSpec) => void
}

export function PipelineEditor({ spec, catalogCache, onChange }: Props) {
  const [mode, setMode] = useState<'visual' | 'yaml'>('visual')
  const [yamlText, setYamlText] = useState(() => specToYaml(spec))
  const [yamlError, setYamlError] = useState<string>()

  function switchToYaml() {
    if (!spec.input || !spec.output) {
      setYamlError('Input und Output müssen belegt sein bevor in den YAML-Modus gewechselt wird.')
      return
    }
    setYamlText(specToYaml(spec))
    setYamlError(undefined)
    setMode('yaml')
  }

  function switchToVisual() {
    try {
      onChange(yamlToSpec(yamlText))
      setYamlError(undefined)
      setMode('visual')
    } catch (e) {
      setYamlError((e as Error).message)
    }
  }

  function handleYamlChange(text: string | undefined) {
    const t = text ?? ''
    setYamlText(t)
    try {
      onChange(yamlToSpec(t))
      setYamlError(undefined)
    } catch {
      // keep spec stable on transient parse errors while typing
    }
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
      <div style={{ marginBottom: 12, display: 'flex', gap: 8 }}>
        <button onClick={switchToVisual} disabled={mode === 'visual'}>
          Visuell
        </button>
        <button onClick={switchToYaml} disabled={mode === 'yaml'}>
          YAML
        </button>
      </div>

      {mode === 'visual' && (
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

      {mode === 'yaml' && (
        <div>
          <Suspense fallback={<div>Lade Editor…</div>}>
            <MonacoEditor
              height="400px"
              language="yaml"
              value={yamlText}
              onChange={handleYamlChange}
              options={{ minimap: { enabled: false }, wordWrap: 'on', fontSize: 13 }}
            />
          </Suspense>
          {yamlError && (
            <p style={{ color: 'red', marginTop: 4, fontSize: 13 }}>{yamlError}</p>
          )}
        </div>
      )}
    </div>
  )
}
