import { describe, expect, it } from 'vitest'
import { GAP_DEADLINE_HOURS, formatRemaining } from './deadline'

describe('gap review deadlines', () => {
  const now = new Date('2026-09-24T12:00:00Z').getTime()
  const dueAt = (deltaMs: number) => new Date(now + deltaMs).toISOString()

  it('uses 4 hours / 1 day / 3 days by severity', () => {
    expect(GAP_DEADLINE_HOURS.critical).toBe(4)
    expect(GAP_DEADLINE_HOURS.major).toBe(24)
    expect(GAP_DEADLINE_HOURS.minor).toBe(72)
  })

  it('formats remaining time in minutes, hours and days', () => {
    expect(formatRemaining(dueAt(30 * 60_000), now)).toMatchObject({ text: '剩 30 分钟', overdue: false })
    expect(formatRemaining(dueAt(3 * 3_600_000), now)).toMatchObject({ text: '剩 3 小时', overdue: false })
    expect(formatRemaining(dueAt(48 * 3_600_000), now)).toMatchObject({ text: '剩 2.0 天', overdue: false })
  })

  it('flags overdue gaps first', () => {
    expect(formatRemaining(dueAt(-5 * 3_600_000), now)).toMatchObject({ text: '已超期 5 小时', overdue: true })
  })
})
