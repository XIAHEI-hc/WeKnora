export const PIXLAB_CHAT_HISTORY_PAGE_SIZE = 50

export type WorkbenchHistoryRow = {
  id?: string
  created_at?: string
}

export function oldestHistoryCursor(rows: WorkbenchHistoryRow[]) {
  return String(rows[0]?.created_at || '')
}

export function prependDistinctHistory<T extends WorkbenchHistoryRow>(current: T[], older: T[]) {
  const existing = new Set(current.map((row) => row.id).filter(Boolean))
  return older.filter((row) => !row.id || !existing.has(row.id))
}

export function historyPageMayHaveOlderRows(rowCount: number, pageSize = PIXLAB_CHAT_HISTORY_PAGE_SIZE) {
  return rowCount === pageSize
}
