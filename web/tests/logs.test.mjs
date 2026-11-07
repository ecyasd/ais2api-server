import assert from 'node:assert/strict'
import test from 'node:test'
import { formatLogLatency, requestLatencyMetrics } from '../src/logs.ts'

function timing(overrides = {}) {
  return {
    stages_ms: {},
    first_thought_ms: null,
    first_body_ms: null,
    first_tool_ms: null,
    generation_ms: null,
    cleanup_ms: null,
    background_cleanup_ms: null,
    output: 'none',
    body_events: 0,
    thought_events: 0,
    tool_events: 0,
    attempts: [],
    ...overrides,
  }
}

test('reasoning before body keeps first event and first response text separate', () => {
  assert.deepEqual(
    requestLatencyMetrics({
      first_event_ms: 250,
      timing: timing({ first_thought_ms: 300, first_body_ms: 3250, output: 'text' }),
    }),
    [
      { label: 'logs.firstEvent', milliseconds: 250 },
      { label: 'logs.firstBody', milliseconds: 3250 },
    ],
  )
})

test('tool-only and thought-only responses never borrow first event time for body', () => {
  for (const output of ['tools', 'thought_only']) {
    const metrics = requestLatencyMetrics({
      first_event_ms: 100,
      timing: timing({ output }),
    })
    assert.deepEqual(metrics, [
      { label: 'logs.firstEvent', milliseconds: 100 },
      { label: 'logs.firstBody', milliseconds: null },
    ])
    assert.equal(formatLogLatency(metrics[1].milliseconds, 'en'), '—')
  }
})

test('older logs show only their measured first event', () => {
  assert.deepEqual(requestLatencyMetrics({ first_event_ms: 1200 }), [
    { label: 'logs.firstEvent', milliseconds: 1200 },
  ])
  assert.deepEqual(requestLatencyMetrics({}), [])
})

test('zero milliseconds remains a valid measurement and missing times remain unknown', () => {
  const metrics = requestLatencyMetrics({
    first_event_ms: 0,
    timing: timing({ first_body_ms: 0, output: 'text' }),
  })
  assert.deepEqual(metrics, [
    { label: 'logs.firstEvent', milliseconds: 0 },
    { label: 'logs.firstBody', milliseconds: 0 },
  ])
  for (const metric of metrics) assert.equal(formatLogLatency(metric.milliseconds, 'en'), '0 s')
  assert.equal(formatLogLatency(null, 'en'), '—')
  assert.equal(formatLogLatency(undefined, 'en'), '—')
})

test('latencies use seconds and the selected locale', () => {
  assert.equal(formatLogLatency(3250, 'en'), '3.25 s')
  assert.equal(formatLogLatency(3250, 'de'), '3,25 s')
})
