import type { GapSeverity } from '../types/enums/gap-severity'

export const GAP_DEADLINE_HOURS: Record<GapSeverity, number> = { critical: 4, major: 24, minor: 72 }

export interface DeadlineView { text: string; overdue: boolean; hours: number }

/** 按“剩余时间”展示复核期限；超期显示为负向提示。 */
export function formatRemaining(dueAt: string | null | undefined, now: number = Date.now()): DeadlineView {
  if (!dueAt) return { text: '无期限', overdue: false, hours: 0 }
  const ms = new Date(dueAt).getTime() - now
  const overdue = ms < 0
  const abs = Math.abs(ms)
  const hours = abs / 3_600_000
  let text: string
  if (hours < 1) {
    const minutes = Math.max(1, Math.round(abs / 60_000))
    text = `${minutes} 分钟`
  } else if (hours < 48) {
    text = `${Math.round(hours)} 小时`
  } else {
    text = `${(hours / 24).toFixed(1)} 天`
  }
  return { text: overdue ? `已超期 ${text}` : `剩 ${text}`, overdue, hours: ms / 3_600_000 }
}
