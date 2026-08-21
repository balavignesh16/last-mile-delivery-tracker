// Shared by QuotePage (M06) and the M07 order pages — every place this
// project displays a computed charge formats it the same way.
export function formatCurrency(amount: number): string {
  return `₹${amount.toFixed(2)}`
}
