import { useState } from 'react'
import { X } from 'lucide-react'
import { ComponentPicker } from './ComponentPicker'
import type { CatalogComponent, ComponentSpec } from '../types'

// SchemaFormComponent is passed as a prop to break the circular import between
// NestedComponentEditor and SchemaForm (NestedComponentEditor ↔ CompositeForm ↔ SchemaForm).
export type SchemaFormComponent = React.ComponentType<{
  component: CatalogComponent
  value: unknown
  catalogCache: Map<string, CatalogComponent>
  depth: number
  onChange: (value: unknown) => void
}>

interface Props {
  kind: 'inputs' | 'processors' | 'outputs'
  items: ComponentSpec[]
  catalogCache: Map<string, CatalogComponent>
  depth: number
  SchemaFormComp: SchemaFormComponent
  onChange: (items: ComponentSpec[]) => void
}

export function NestedComponentEditor({
  kind,
  items,
  catalogCache,
  depth,
  SchemaFormComp,
  onChange,
}: Props) {
  const [picking, setPicking] = useState(false)

  function handleSelect(comp: CatalogComponent) {
    const isDirectArray =
      comp.bodyKind === 'composite' &&
      comp.compositeFields?.length === 1 &&
      comp.compositeFields[0].field === ''

    const newSpec: ComponentSpec = {
      type: comp.name,
      config: comp.bodyKind === 'scalar' ? '' : isDirectArray ? [] : {},
    }
    onChange([...items, newSpec])
    setPicking(false)
  }

  function handleConfigChange(idx: number, config: unknown) {
    onChange(items.map((it, i) => (i === idx ? { ...it, config } : it)))
  }

  function handleRemove(idx: number) {
    onChange(items.filter((_, i) => i !== idx))
  }

  const indent = depth * 16

  return (
    <div
      style={{
        marginLeft: indent,
        borderLeft: depth > 0 ? '2px solid #e0e7ff' : 'none',
        paddingLeft: depth > 0 ? 12 : 0,
      }}
    >
      {items.map((item, idx) => {
        const comp = catalogCache.get(kind + '/' + item.type)
        return (
          <div
            key={idx}
            style={{
              marginBottom: 8,
              background: '#f8f9ff',
              borderRadius: 4,
              padding: 10,
              border: '1px solid #dde',
            }}
          >
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 4,
              }}
            >
              <strong style={{ fontSize: 13 }}>{item.type}</strong>
              <button
                onClick={() => handleRemove(idx)}
                style={{ color: '#c00', border: 'none', background: 'none', cursor: 'pointer', padding: 0, display: 'flex' }}
              >
                <X size={14} />
              </button>
            </div>
            {comp && (
              <SchemaFormComp
                component={comp}
                value={item.config}
                catalogCache={catalogCache}
                depth={depth + 1}
                onChange={val => handleConfigChange(idx, val)}
              />
            )}
          </div>
        )
      })}
      <button onClick={() => setPicking(true)} style={addBtnStyle}>
        + {kind.slice(0, -1)} hinzufügen
      </button>
      {picking && (
        <ComponentPicker
          category={kind}
          onSelect={handleSelect}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  )
}

const addBtnStyle: React.CSSProperties = {
  padding: '4px 12px',
  cursor: 'pointer',
  borderRadius: 4,
  border: '1px dashed #aab',
  background: 'none',
  fontSize: 12,
  marginTop: 4,
}
