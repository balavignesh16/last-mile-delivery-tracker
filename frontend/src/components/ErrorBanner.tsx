export function ErrorBanner({ message }: { message: string | null }) {
  if (!message) return null
  return (
    <div role="alert" className="rounded-md border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800">
      {message}
    </div>
  )
}
