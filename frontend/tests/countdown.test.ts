import assert from 'node:assert/strict'
import test from 'node:test'
import { formatCountdown } from '../src/components/CountdownTimer/format.ts'

test('countdown switches units at day and hour boundaries', () => {
  for (const [seconds, expected] of [
    [1209600, '14:00:00:00'],
    [604800, '07:00:00:00'],
    [604799, '06:23:59:59'],
    [86401, '01:00:00:01'],
    [86400, '01:00:00:00'],
    [86399, '23:59:59'],
    [5400, '01:30:00'],
    [3600, '01:00:00'],
    [3601, '01:00:01'],
    [3599, '59:59'],
    [60, '01:00'],
    [59.9, '00:59'],
    [1, '00:01'],
    [0, '00:00'],
    [-1, '00:00'],
  ] as const) {
    assert.equal(formatCountdown(seconds), expected)
  }
})
