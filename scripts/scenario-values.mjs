// Resolve input references against one snapshot before submitting a command/batch.
export function resolveValues(value, states) {
  if (Array.isArray(value)) return value.map(item => resolveValues(item, states));
  if (value === null || typeof value !== 'object') return value;
  if (Object.hasOwn(value, '$state')) {
    const ref = value.$state;
    if (Object.keys(value).length !== 1 || !ref || typeof ref.node !== 'string' || !ref.node ||
        typeof ref.path !== 'string' || !ref.path) {
      throw new Error('Invalid $state reference: expected {"$state":{"node":"node-1","path":"clock"}}');
    }
    let current = Object.hasOwn(states, ref.node) ? states[ref.node] : undefined;
    for (const key of ref.path.split('.')) {
      current = current !== null && typeof current === 'object' && Object.hasOwn(current, key) ? current[key] : undefined;
    }
    if (current === undefined) throw new Error(`Missing $state value: ${ref.node}.${ref.path}`);
    // Return the recorded data, not another template, without sharing mutable arrays.
    return structuredClone(current);
  }
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, resolveValues(item, states)]));
}
