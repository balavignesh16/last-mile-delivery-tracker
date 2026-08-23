import type { ReactNode } from 'react'
import { useInView } from '../hooks/useInView'

// Subtle fade/translate reveal on scroll. Content is always present in
// the DOM (never display:none/visibility:hidden), so it's unaffected by
// JavaScript being unavailable or slow to run — only the opacity/
// transform animate, and index.css's global prefers-reduced-motion rule
// already collapses that transition to effectively instant.
export function Reveal({ children, className = '' }: { children: ReactNode; className?: string }) {
  const [ref, inView] = useInView<HTMLDivElement>()

  return (
    <div
      ref={ref}
      className={`transition-all duration-700 ease-out ${inView ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-4'} ${className}`}
    >
      {children}
    </div>
  )
}
