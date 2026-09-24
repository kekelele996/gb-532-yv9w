import type { CoverageGap } from '../types/coverage-gap'

export interface DeadlineView { remainMs:number; overdue:boolean; label:string }

export function deadlineView(gap:Pick<CoverageGap,'deadline_at'>, now:Date=new Date()):DeadlineView {
  const deadline = new Date(gap.deadline_at)
  if (Number.isNaN(deadline.getTime())) {
    return { remainMs: Number.POSITIVE_INFINITY, overdue: false, label: '期限待计算' }
  }
  const remainMs = deadline.getTime() - now.getTime()
  return { remainMs, overdue: remainMs <= 0, label: formatRemain(remainMs) }
}

export function formatRemain(remainMs:number):string {
  const overdue = remainMs < 0
  const abs = Math.abs(remainMs)
  const totalMinutes = Math.floor(abs / 60_000)
  const days = Math.floor(totalMinutes / (60 * 24))
  const hours = Math.floor((totalMinutes % (60 * 24)) / 60)
  const minutes = totalMinutes % 60
  const parts:string[] = []
  if (days > 0) parts.push(`${days} 天`)
  if (hours > 0 || days > 0) parts.push(`${hours} 小时`)
  if (days === 0) parts.push(`${minutes} 分钟`)
  return overdue ? `已超期 ${parts.join(' ')}` : `剩余 ${parts.join(' ')}`
}
