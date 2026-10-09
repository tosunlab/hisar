export const minimumVisibleMilliseconds = 300;

export class VisibilityGuard {
  private visibleSince: number | undefined;

  constructor(private readonly minimumMilliseconds = minimumVisibleMilliseconds) {}

  recordVisibility(isVisible: boolean, now: number): void {
    if (!isVisible) {
      this.visibleSince = undefined;
      return;
    }
    this.visibleSince ??= now;
  }

  isInteractionAllowed(now: number): boolean {
    return this.visibleSince !== undefined && now - this.visibleSince >= this.minimumMilliseconds;
  }

  reset(): void {
    this.visibleSince = undefined;
  }
}
