import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { OrderStatus } from '../types/order'
import { OrderStatusBadge } from './OrderStatusBadge'

const EXPECTED_LABELS: Record<OrderStatus, string> = {
  CREATED: 'Created',
  ASSIGNED: 'Assigned',
  PICKED_UP: 'Picked up',
  IN_TRANSIT: 'In transit',
  OUT_FOR_DELIVERY: 'Out for delivery',
  DELIVERED: 'Delivered',
  FAILED: 'Failed',
  RESCHEDULED: 'Rescheduled',
}

describe('OrderStatusBadge', () => {
  it('renders the correct human-readable label for every order status', () => {
    for (const [status, label] of Object.entries(EXPECTED_LABELS) as [OrderStatus, string][]) {
      const { unmount } = render(<OrderStatusBadge status={status} />)
      expect(screen.getByText(label)).toBeTruthy()
      unmount()
    }
  })

  it('gives DELIVERED and FAILED visually distinct (non-identical) styling', () => {
    const { container: delivered, unmount: unmountDelivered } = render(<OrderStatusBadge status="DELIVERED" />)
    const deliveredClass = delivered.querySelector('span')?.className
    unmountDelivered()

    const { container: failed, unmount: unmountFailed } = render(<OrderStatusBadge status="FAILED" />)
    const failedClass = failed.querySelector('span')?.className
    unmountFailed()

    expect(deliveredClass).not.toBe(failedClass)
  })

  it('gives ASSIGNED and IN_TRANSIT visually distinct styling (not collapsed into one "in progress" bucket)', () => {
    const { container: assigned, unmount: unmountAssigned } = render(<OrderStatusBadge status="ASSIGNED" />)
    const assignedClass = assigned.querySelector('span')?.className
    unmountAssigned()

    const { container: inTransit, unmount: unmountInTransit } = render(<OrderStatusBadge status="IN_TRANSIT" />)
    const inTransitClass = inTransit.querySelector('span')?.className
    unmountInTransit()

    expect(assignedClass).not.toBe(inTransitClass)
  })
})
