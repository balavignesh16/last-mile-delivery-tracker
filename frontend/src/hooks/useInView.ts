import { useEffect, useRef, useState, type RefObject } from 'react'

// Drives scroll-reveal sections via IntersectionObserver — no animation
// library. Defensive default: if IntersectionObserver isn't available at
// all, content starts (and stays) visible rather than being trapped
// hidden. Reveals are one-shot — once an element has been seen, the
// observer disconnects rather than toggling back out on scroll-away,
// which would read as a distracting repeating effect.
export function useInView<T extends HTMLElement>(): [RefObject<T | null>, boolean] {
  const ref = useRef<T>(null)
  const [inView, setInView] = useState(() => typeof IntersectionObserver === 'undefined')

  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') return
    const el = ref.current
    if (!el) return

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true)
          observer.disconnect()
        }
      },
      { threshold: 0.15, rootMargin: '0px 0px -60px 0px' },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  return [ref, inView]
}
