import { Check, ChevronDown } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

export interface SelectOption {
  value: string
  label: string
  disabled?: boolean
}

interface SelectProps {
  id?: string
  'aria-label'?: string
  value: string
  onChange: (value: string) => void
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  className?: string
}

// A themed replacement for native <select>, implementing the WAI-ARIA
// "listbox button" pattern: focus stays on the trigger the whole time
// (aria-activedescendant announces the highlighted option), so all
// keyboard handling lives in one place instead of needing to move real
// DOM focus into the popup list. No portal/collision-avoidance — every
// call site in this app is a normal in-page form field, never near a
// viewport edge.
//
// Deliberately renders no <label> of its own — every call site already
// has its own external <label htmlFor={id}> (or passes aria-label for
// the one site that never had a visible label), same as when these were
// native <select> elements; adding a second label here would just
// duplicate the accessible name.
export function Select({ id, 'aria-label': ariaLabel, value, onChange, options, placeholder, disabled, className }: SelectProps) {
  const [open, setOpen] = useState(false)
  const [highlighted, setHighlighted] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  // The placeholder (if any) is just an ordinary, always-first option
  // with value '' — this keeps every index-based calculation below
  // (selection, keyboard highlight, commit) working over one flat list
  // instead of juggling an offset.
  const items: SelectOption[] = placeholder ? [{ value: '', label: placeholder }, ...options] : options
  const selectedIndex = items.findIndex((o) => o.value === value)
  const selected = selectedIndex >= 0 ? items[selectedIndex] : null
  const showPlaceholder = !selected || selected.value === ''

  useEffect(() => {
    if (!open) return
    function handleClickOutside(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [open])

  function openList() {
    if (disabled || items.length === 0) return
    setHighlighted(selectedIndex >= 0 ? selectedIndex : 0)
    setOpen(true)
  }

  function closeList() {
    setOpen(false)
    triggerRef.current?.focus()
  }

  function commit(index: number) {
    const option = items[index]
    if (!option || option.disabled) return
    onChange(option.value)
    setOpen(false)
  }

  function moveHighlight(delta: number) {
    setHighlighted((prev) => {
      let next = prev
      for (let i = 0; i < items.length; i++) {
        next = (next + delta + items.length) % items.length
        if (!items[next].disabled) return next
      }
      return prev
    })
  }

  function handleTriggerKeyDown(e: React.KeyboardEvent) {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        openList()
      }
      return
    }
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        moveHighlight(1)
        break
      case 'ArrowUp':
        e.preventDefault()
        moveHighlight(-1)
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        commit(highlighted)
        triggerRef.current?.focus()
        break
      case 'Escape':
        e.preventDefault()
        closeList()
        break
      case 'Tab':
        setOpen(false)
        break
    }
  }

  const listboxId = id ? `${id}-listbox` : undefined
  const triggerLabel = showPlaceholder ? (placeholder ?? 'Select…') : (selected?.label ?? '')

  return (
    <div ref={rootRef} className={`relative ${className ?? ''}`}>
      <button
        ref={triggerRef}
        type="button"
        id={id}
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-activedescendant={open && items[highlighted] ? `${id}-option-${highlighted}` : undefined}
        disabled={disabled}
        onClick={() => (open ? setOpen(false) : openList())}
        onKeyDown={handleTriggerKeyDown}
        className="flex w-full items-center justify-between gap-2 rounded-md border border-slate-300 bg-white px-3 py-2 text-left text-sm text-slate-900 transition-colors disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-400"
      >
        <span className={`truncate ${showPlaceholder ? 'text-slate-400' : ''}`}>{triggerLabel}</span>
        <ChevronDown className={`h-4 w-4 shrink-0 text-slate-400 transition-transform ${open ? 'rotate-180' : ''}`} aria-hidden="true" />
      </button>

      {open && (
        <ul
          id={listboxId}
          role="listbox"
          tabIndex={-1}
          aria-label={ariaLabel}
          className="absolute top-full left-0 z-20 mt-1 max-h-60 w-full overflow-auto rounded-md border border-navy-100 bg-white py-1 text-sm shadow-lg"
        >
          {items.map((option, index) => {
            const isSelected = index === selectedIndex
            return (
              <li
                key={option.value}
                id={id ? `${id}-option-${index}` : undefined}
                role="option"
                aria-selected={isSelected}
                aria-disabled={option.disabled}
                onMouseEnter={() => !option.disabled && setHighlighted(index)}
                onClick={() => commit(index)}
                className={`flex items-center justify-between gap-2 px-3 py-2 ${
                  option.disabled
                    ? 'cursor-not-allowed text-slate-300'
                    : `cursor-pointer ${isSelected ? 'bg-navy-50 font-medium text-navy-700' : `text-slate-900 ${highlighted === index ? 'bg-navy-50' : ''}`}`
                }`}
              >
                {option.label}
                {isSelected && <Check className="h-4 w-4 shrink-0 text-navy-600" aria-hidden="true" />}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
