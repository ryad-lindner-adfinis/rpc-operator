import { useState } from 'react'
import { ComponentPicker } from './ComponentPicker'
import { SchemaForm } from './SchemaForm'
import type { CatalogComponent, ComponentSpec } from '../types'

interface Props {
  title: string
  category: 'inputs' | 'processors' | 'outputs'
  multi?: boolean
  items: ComponentSpec[]
  catalogCache: Map<string, CatalogComponent>
  onChange: (items: ComponentSpec[]) => void
}

export function ComponentBox({ title, category, multi, items, catalogCache, onChange }: Props) {
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
    onChange(multi ? [...items, newSpec] : [newSpec])
    setPicking(false)
  }

  function handleConfigChange(idx: number, config: unknown) {
    onChange(items.map((it, i) => (i === idx ? { ...it, config } : it)))
  }

  function handleRemove(idx: number) {
    onChange(items.filter((_, i) => i !== idx))
  }

  return (
    <div style={boxStyle}>
      <h4 style={{ margin: '0 0 12px', color: '#333' }}>{title}</h4>
      {items.map((item, idx) => {
        const comp = catalogCache.get(category + '/' + item.type)
        return (
          <div key={idx} style={slotStyle}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 8,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <strong>{item.type}</strong>
                {comp?.bodyKind === 'composite' && (
                  <span
                    style={{
                      fontSize: 11,
                      color: '#3b5',
                      background: '#e8fff0',
                      padding: '1px 5px',
                      borderRadius: 8,
                    }}
                  >
                    composite
                  </span>
                )}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                {!multi && (
                  <button
                    onClick={() => setPicking(true)}
                    style={{
                      color: '#555', border: '1px solid #ccc', background: 'none',
                      cursor: 'pointer', borderRadius: 3, fontSize: 12, padding: '1px 8px',
                    }}
                  >
                    Ändern
                  </button>
                )}
                <button
                  onClick={() => handleRemove(idx)}
                  style={{ color: '#c00', border: 'none', background: 'none', cursor: 'pointer' }}
                >
                  ✕
                </button>
              </div>
            </div>
            {comp && (
              <SchemaForm
                component={comp}
                value={item.config}
                catalogCache={catalogCache}
                depth={0}
                onChange={val => handleConfigChange(idx, val)}
              />
            )}
          </div>
        )
      })}
      {(multi || items.length === 0) && (
        <button onClick={() => setPicking(true)} style={addBtnStyle}>
          + hinzufügen
        </button>
      )}
      {picking && (
        <ComponentPicker
          category={category}
          onSelect={handleSelect}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  )
}

const boxStyle: React.CSSProperties = {
  flex: 1,
  border: '2px solid #dde',
  borderRadius: 8,
  padding: 16,
  minWidth: 260,
}
const slotStyle: React.CSSProperties = {
  background: '#f8f9ff',
  borderRadius: 4,
  padding: 12,
  marginBottom: 8,
}
const addBtnStyle: React.CSSProperties = {
  marginTop: 8,
  padding: '6px 14px',
  cursor: 'pointer',
  borderRadius: 4,
  border: '1px dashed #aab',
  background: 'none',
  width: '100%',
}
