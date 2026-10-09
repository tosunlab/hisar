import type { InlineMenuPurpose } from "../shared/messages";

export interface InlineMenuSession {
  sessionID: string;
  tabId: number;
  frameId: number;
  frameUrl: string;
  purpose: InlineMenuPurpose;
  menuFrameId?: number;
  suggestedPassword?: string;
  createdAt: number;
}

export class InlineMenuSessionStore {
  private readonly allSession = new Map<string, InlineMenuSession>();

  constructor(
    private readonly now: () => number = () => Date.now(),
    private readonly lifetimeMilliseconds = 30 * 60 * 1000,
  ) {}

  create(session: Omit<InlineMenuSession, "createdAt">): InlineMenuSession {
    for (const existing of this.allSession.values()) {
      if (existing.tabId === session.tabId && existing.frameId === session.frameId) this.allSession.delete(existing.sessionID);
    }
    const created = { ...session, createdAt: this.now() };
    this.allSession.set(session.sessionID, created);
    return created;
  }

  get(sessionID: string): InlineMenuSession | undefined {
    const session = this.allSession.get(sessionID);
    if (session && this.now() - session.createdAt > this.lifetimeMilliseconds) {
      this.allSession.delete(sessionID);
      return undefined;
    }
    return session;
  }

  claim(sessionID: string, tabId: number, menuFrameId: number): InlineMenuSession | undefined {
    const session = this.get(sessionID);
    if (!session || session.tabId !== tabId) return undefined;
    if (session.menuFrameId === undefined) session.menuFrameId = menuFrameId;
    return session.menuFrameId === menuFrameId ? session : undefined;
  }

  remove(sessionID: string): void {
    this.allSession.delete(sessionID);
  }

  removeTab(tabId: number): void {
    for (const session of this.allSession.values()) {
      if (session.tabId === tabId) this.allSession.delete(session.sessionID);
    }
  }
}
