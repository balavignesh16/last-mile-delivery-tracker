import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Select } from './Select'

const OPTIONS = [
  { value: 'a', label: 'Option A' },
  { value: 'b', label: 'Option B' },
  { value: 'c', label: 'Option C', disabled: true },
]

describe('Select', () => {
  it('shows the placeholder when no value is selected', () => {
    render(<Select id="s" value="" onChange={vi.fn()} options={OPTIONS} placeholder="Choose one…" />)
    expect(screen.getByRole('button', { name: 'Choose one…' })).toBeTruthy()
  })

  it('shows the selected option label on the trigger', () => {
    render(<Select id="s" value="b" onChange={vi.fn()} options={OPTIONS} placeholder="Choose one…" />)
    expect(screen.getByRole('button', { name: 'Option B' })).toBeTruthy()
  })

  it('opens the listbox on click and shows every option', () => {
    render(<Select id="s" value="" onChange={vi.fn()} options={OPTIONS} />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('listbox')).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Option A' })).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Option B' })).toBeTruthy()
    expect(screen.getByRole('option', { name: 'Option C' })).toBeTruthy()
  })

  it('calls onChange and closes when an option is clicked', () => {
    const onChange = vi.fn()
    render(<Select id="s" value="" onChange={onChange} options={OPTIONS} />)
    fireEvent.click(screen.getByRole('button'))
    fireEvent.click(screen.getByRole('option', { name: 'Option A' }))
    expect(onChange).toHaveBeenCalledWith('a')
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('does not select a disabled option', () => {
    const onChange = vi.fn()
    render(<Select id="s" value="" onChange={onChange} options={OPTIONS} />)
    fireEvent.click(screen.getByRole('button'))
    fireEvent.click(screen.getByRole('option', { name: 'Option C' }))
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('listbox')).toBeTruthy()
  })

  it('opens with ArrowDown and selects the highlighted option with Enter', () => {
    const onChange = vi.fn()
    render(<Select id="s" value="" onChange={onChange} options={OPTIONS} />)
    const trigger = screen.getByRole('button')
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(screen.getByRole('listbox')).toBeTruthy()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    fireEvent.keyDown(trigger, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('b')
  })

  it('closes without changing the value on Escape', () => {
    const onChange = vi.fn()
    render(<Select id="s" value="a" onChange={onChange} options={OPTIONS} />)
    const trigger = screen.getByRole('button')
    fireEvent.click(trigger)
    fireEvent.keyDown(trigger, { key: 'Escape' })
    expect(onChange).not.toHaveBeenCalled()
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('closes when clicking outside', () => {
    render(
      <div>
        <Select id="s" value="" onChange={vi.fn()} options={OPTIONS} />
        <button type="button">outside</button>
      </div>,
    )
    fireEvent.click(screen.getByRole('button', { name: /select…/i }))
    expect(screen.getByRole('listbox')).toBeTruthy()
    fireEvent.mouseDown(screen.getByRole('button', { name: 'outside' }))
    expect(screen.queryByRole('listbox')).toBeNull()
  })

  it('does not open when disabled', () => {
    render(<Select id="s" value="" onChange={vi.fn()} options={OPTIONS} disabled />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.queryByRole('listbox')).toBeNull()
  })
})
