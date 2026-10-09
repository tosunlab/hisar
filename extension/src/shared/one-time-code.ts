export function formatOneTimeCode(code: string): string {
  const middle = Math.ceil(code.length / 2);
  return `${code.slice(0, middle)} ${code.slice(middle)}`.trim();
}

export function computeSecondsLeft(expiresAt: string, now: number): number {
  const expiresAtMilliseconds = Date.parse(expiresAt);
  return Number.isNaN(expiresAtMilliseconds) ? 0 : Math.max(0, Math.ceil((expiresAtMilliseconds - now) / 1000));
}
