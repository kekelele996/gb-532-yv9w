import { describe, expect, it } from 'vitest'
import { deadlineView, formatRemain } from './deadline'

describe('gap deadline formatting', () => {
  const now = new Date('2026-09-24T08:00:00Z')
  it('formats remaining hours for critical gaps', () => {
    const view = deadlineView({ deadline_at: '2026-09-24T12:00:00Z' }, now)
    expect(view.overdue).toBe(false)
    expect(view.remainMs).toBe(4 * 60 * 60 * 1000)
    expect(view.label).toBe('剩余 4 小时 0 分钟')
  })
  it('formats multi-day deadlines for minor gaps', () => {
    expect(formatRemain(3 * 24 * 60 * 60 * 1000 - 60 * 60 * 1000)).toBe('剩余 2 天 23 小时')
  })
  it('flags overdue gaps', () => {
    const view = deadlineView({ deadline_at: '2026-09-24T07:30:00Z' }, now)
    expect(view.overdue).toBe(true)
    expect(view.label).toBe('已超期 30 分钟')
  })
})
