import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolveValues } from '../scripts/scenario-values.mjs';

const timestamp = { lamport: 2, vector: [2, 0, 0], total: 6 };
const states = { 'node-1': { clock: timestamp, events: [{ timestamp }], zero: 0, flag: false, empty: '', nil: null } };
const ref = path => ({ $state: { node: 'node-1', path } });

test('resolves nested timestamp references and array indices without mutating state or input', () => {
  const input = { left: ref('events.0.timestamp'), nested: [ref('clock'), 0, false, null] };
  const before = structuredClone(input);
  const result = resolveValues(input, states);
  assert.deepEqual(result, { left: timestamp, nested: [timestamp, 0, false, null] });
  result.left.vector[0] = 99;
  assert.equal(states['node-1'].clock.vector[0], 2);
  assert.deepEqual(input, before);
});

test('preserves ordinary inputs and referenced zero, false, empty string, and null', () => {
  const input = { key: 'x', values: [0, false, '', null], nested: { count: 6 } };
  assert.deepEqual(resolveValues(input, states), input);
  assert.deepEqual(resolveValues(['zero', 'flag', 'empty', 'nil'].map(ref), states), [0, false, '', null]);
});

test('rejects missing, inherited, and malformed references instead of posting empty timestamps', () => {
  for (const value of [ref('missing'), ref('events.2.timestamp'), ref('toString'),
    { $state: { node: 'absent', path: 'clock' } }, { $state: null },
    { $state: { node: 'node-1', path: 1 } }, { ...ref('clock'), extra: true }]) {
    assert.throws(() => resolveValues(value, states), /\$state/);
  }
});

test('all logical-clock scenario comparison inputs resolve to concrete timestamps', () => {
  const spec = JSON.parse(readFileSync(new URL('../protocols/logical-clocks/scenarios.json', import.meta.url)));
  const snapshot = Object.fromEntries(['node-1', 'node-2', 'node-3'].map(node => [node, {
    clock: timestamp, events: [{ timestamp }, { timestamp }],
  }]));
  let comparisons = 0;
  for (const scenario of spec.scenarios) {
    for (const step of scenario.steps) {
      for (const input of step.concurrent || (step.input ? [step.input] : [])) {
        const values = resolveValues(input.values || {}, snapshot);
        if (input.method === 'Application.CompareEvents') {
          comparisons++;
          assert.deepEqual(values.left, timestamp);
          assert.deepEqual(values.right, timestamp);
        }
      }
    }
  }
  assert.equal(comparisons, 9);
});
