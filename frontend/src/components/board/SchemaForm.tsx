import type { JSONSchema, JSONSchemaProperty } from '../../api/types'

interface SchemaFormProps {
  schema: JSONSchema
  value: Record<string, unknown>
  onChange: (next: Record<string, unknown>) => void
}

// Turns a snake_case / camelCase key into a human label ("sprint_length_days" → "Sprint length days").
function humanize(key: string): string {
  const spaced = key.replace(/[_-]+/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

/**
 * Renders an editable form for a board type's `config_schema`. Supports the common
 * JSON-Schema subset the Board Registry uses: object properties with string /
 * integer / number / boolean types, enums (→ select), and min/max on numbers.
 * Unsupported constructs are skipped, so a new board type with a richer schema
 * degrades gracefully rather than breaking the create dialog.
 */
export default function SchemaForm({ schema, value, onChange }: SchemaFormProps) {
  const properties = schema?.properties ?? {}
  const keys = Object.keys(properties)
  const required = new Set(schema?.required ?? [])

  if (keys.length === 0) return null

  function setField(key: string, fieldValue: unknown) {
    onChange({ ...value, [key]: fieldValue })
  }

  return (
    <div className="space-y-4">
      {keys.map((key) => {
        const prop = properties[key]
        return (
          <Field
            key={key}
            name={key}
            prop={prop}
            required={required.has(key)}
            value={value[key]}
            onChange={(v) => setField(key, v)}
          />
        )
      })}
    </div>
  )
}

interface FieldProps {
  name: string
  prop: JSONSchemaProperty
  required: boolean
  value: unknown
  onChange: (v: unknown) => void
}

function Field({ name, prop, required, value, onChange }: FieldProps) {
  const label = prop.title ?? humanize(name)

  // Enum → select.
  if (prop.enum && prop.enum.length > 0) {
    return (
      <div>
        <FieldLabel label={label} required={required} description={prop.description} />
        <select
          className="input-base w-full"
          value={value === undefined || value === null ? '' : String(value)}
          onChange={(e) => onChange(coerce(e.target.value, prop.type))}
        >
          {!required && <option value="">—</option>}
          {prop.enum.map((opt) => (
            <option key={String(opt)} value={String(opt)}>{String(opt)}</option>
          ))}
        </select>
      </div>
    )
  }

  // Boolean → checkbox.
  if (prop.type === 'boolean') {
    return (
      <label className="flex items-center gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={Boolean(value)}
          onChange={(e) => onChange(e.target.checked)}
          className="rounded border-border-2"
        />
        <span className="text-sm text-text-1">{label}</span>
        {prop.description && <span className="text-xs text-text-3">— {prop.description}</span>}
      </label>
    )
  }

  // Integer / number → numeric input.
  if (prop.type === 'integer' || prop.type === 'number') {
    return (
      <div>
        <FieldLabel label={label} required={required} description={prop.description} />
        <input
          type="number"
          className="input-base w-full"
          value={value === undefined || value === null ? '' : Number(value)}
          min={prop.minimum}
          max={prop.maximum}
          step={prop.type === 'integer' ? 1 : 'any'}
          onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
        />
      </div>
    )
  }

  // Default: string text input.
  return (
    <div>
      <FieldLabel label={label} required={required} description={prop.description} />
      <input
        type="text"
        className="input-base w-full"
        value={value === undefined || value === null ? '' : String(value)}
        onChange={(e) => onChange(e.target.value === '' ? undefined : e.target.value)}
      />
    </div>
  )
}

function FieldLabel({ label, required, description }: { label: string; required: boolean; description?: string }) {
  return (
    <label className="label block mb-1.5">
      {label}{required && <span className="text-danger"> *</span>}
      {description && <span className="font-normal text-text-3"> — {description}</span>}
    </label>
  )
}

function coerce(raw: string, type?: string): unknown {
  if (raw === '') return undefined
  if (type === 'integer' || type === 'number') return Number(raw)
  return raw
}
