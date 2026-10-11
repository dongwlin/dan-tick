// Table rows have a 72px minimum height and the sticky header occupies 44px.
// Reserve the footer only if the full result cannot fit without pagination.
export function historyPageSize(availableHeight: number, total: number): number {
  if (availableHeight <= 0) return 6 // Initial query before desktop layout is measured.
  const capacity = (footer: number) =>
    Math.max(2, Math.min(100, Math.floor((availableHeight - 44 - footer - 2) / 72)))
  const withoutFooter = capacity(0)
  return total > withoutFooter ? capacity(54) : withoutFooter
}
