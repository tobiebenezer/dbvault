export function nextReconnectDelay(attempt) {
  const steps = [1000, 2000, 5000, 10000, 15000, 30000];
  return steps[Math.min(Math.max(0, attempt), steps.length - 1)];
}
