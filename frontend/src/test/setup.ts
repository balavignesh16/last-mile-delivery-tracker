import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// React Testing Library does not auto-unmount between tests under Vitest
// without this — without it, every render() in a file leaves its DOM tree
// behind, and later assertions in the same file start matching multiple
// stale copies of the same data-testid.
afterEach(() => {
  cleanup()
})
