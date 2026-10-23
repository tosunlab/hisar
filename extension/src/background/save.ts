import type { SaveOfferKind, SaveOfferView } from "../shared/messages";
import type { CheckResult, VaultSummary } from "../shared/protocol";

export function decideSaveOffer(checkResult: CheckResult, isNeverOrigin: boolean): SaveOfferKind | undefined {
  if (isNeverOrigin) return undefined;
  if (checkResult.status === "new") return "save";
  if (checkResult.status === "changed" && checkResult.vaultID && checkResult.itemID) return "update";
  return undefined;
}

// pairingToken binds an offer to the pairing it was made under: a browser paired again, as after another account signs
// in to Hisar, never sends the earlier account's password.
export interface SaveOffer {
  offerID: string;
  pairingToken: string;
  saveContext: string;
  navigationID?: string;
  vaultName?: string;
  isPersonal?: boolean;
  tabId: number;
  url: string;
  host: string;
  username: string;
  password: string;
  kind: SaveOfferKind;
  vaultID?: string;
  itemID?: string;
  title?: string;
  defaultTitle?: string;
  allVault?: VaultSummary[];
  createdAt: number;
}

export function toSaveOfferView(offer: SaveOffer): SaveOfferView {
  const view: SaveOfferView = { offerID: offer.offerID, kind: offer.kind, host: offer.host, username: offer.username, title: offer.title, vaultName: offer.vaultName, isPersonal: offer.isPersonal };
  if (offer.allVault) {
    view.defaultTitle = offer.defaultTitle;
    view.allVault = offer.allVault;
  }
  return view;
}

export type ScheduleSaveExpiry = (tabId: number, generation: string, deadline: number, expire: () => void) => () => void;

export class SaveOfferStore {
  private readonly allOfferByTab = new Map<number, SaveOffer>();
  private readonly allNavigation = new Map<number, { id: string; url: string; sourceURL: string; frameId: number; documentID?: string; createdAt: number; didNavigate: boolean }>();
  private readonly allExpiry = new Map<number, { generation: string; deadline: number; cancel: () => void }>();

  constructor(
    private readonly now: () => number = () => Date.now(),
    private readonly lifetimeMilliseconds = 120000,
    private readonly onRemoved: (tabId: number) => void = () => {},
    private readonly scheduleExpiry?: ScheduleSaveExpiry,
  ) {}

  clear(): void {
    for (const tabId of this.allExpiry.keys()) this.removeTab(tabId);
  }

  beginSubmission(tabId: number, id: string, url: string, frameId = 0, sourceURL = url, documentID?: string, createdAt = this.now()): void {
    this.removeTab(tabId);
    this.allNavigation.set(tabId, { id, url, sourceURL, frameId, documentID, createdAt, didNavigate: false });
    this.armExpiry(tabId, id, createdAt);
  }

  navigate(tabId: number, url: string, frameId = 0, documentID?: string): void {
    const navigation = this.allNavigation.get(tabId);
    if (!navigation || frameId !== 0 && frameId !== navigation.frameId) return;
    const previousURL = frameId === 0 ? navigation.url : navigation.sourceURL;
    if (!navigation.didNavigate && this.now() - navigation.createdAt < 10000 && new URL(previousURL).origin === new URL(url).origin) {
      if (frameId === 0) navigation.url = url;
      navigation.sourceURL = url;
      if (frameId === navigation.frameId && documentID) navigation.documentID = documentID;
      navigation.didNavigate = true;
      return;
    }
    this.removeTab(tabId);
  }

  isCurrent(tabId: number, id: string, url?: string): boolean {
    const navigation = this.allNavigation.get(tabId);
    return navigation?.id === id && (url === undefined || navigation.url === url);
  }

  matchesDocument(tabId: number, id: string, documentID: string | undefined): boolean {
    const navigation = this.allNavigation.get(tabId);
    return navigation?.id === id && navigation.documentID === documentID;
  }

  put(offer: SaveOffer): boolean {
    if (this.now() - offer.createdAt >= this.lifetimeMilliseconds || offer.navigationID && !this.isCurrent(offer.tabId, offer.navigationID)) return false;
    if (!offer.navigationID) {
      this.removeTab(offer.tabId);
      this.armExpiry(offer.tabId, offer.offerID, offer.createdAt);
    }
    this.allOfferByTab.set(offer.tabId, offer);
    return true;
  }

  getForTab(tabId: number, pairingToken: string | undefined): SaveOffer | undefined {
    const offer = this.allOfferByTab.get(tabId);
    if (!offer) return undefined;
    if (offer.pairingToken !== pairingToken || this.now() - offer.createdAt >= this.lifetimeMilliseconds) {
      this.removeTab(tabId);
      return undefined;
    }
    return offer;
  }

  take(tabId: number, offerID: string): SaveOffer | undefined {
    const offer = this.allOfferByTab.get(tabId);
    if (!offer || offer.offerID !== offerID) return undefined;
    this.removeTab(tabId);
    return offer;
  }

  removeTab(tabId: number): void {
    this.allOfferByTab.delete(tabId);
    this.allNavigation.delete(tabId);
    const expiry = this.allExpiry.get(tabId);
    if (expiry) {
      expiry.cancel();
      this.allExpiry.delete(tabId);
      this.onRemoved(tabId);
    }
  }

  private armExpiry(tabId: number, generation: string, createdAt: number): void {
    const deadline = createdAt + this.lifetimeMilliseconds;
    const expire = () => {
      const current = this.allExpiry.get(tabId);
      if (current?.generation === generation && this.now() >= current.deadline) this.removeTab(tabId);
    };
    let cancel: () => void;
    if (this.scheduleExpiry) {
      cancel = this.scheduleExpiry(tabId, generation, deadline, expire);
    } else {
      const timer = setTimeout(expire, Math.max(0, deadline - this.now()));
      cancel = () => clearTimeout(timer);
    }
    this.allExpiry.set(tabId, { generation, deadline, cancel });
  }
}

interface RememberedUsername {
  origin: string;
  pairingToken: string;
  username: string;
  createdAt: number;
}

export class RememberedUsernameStore {
  private readonly allUsernameByTab = new Map<number, RememberedUsername>();

  constructor(
    private readonly now: () => number = () => Date.now(),
    private readonly lifetimeMilliseconds = 600000,
  ) {}

  remember(tabId: number, origin: string, username: string, pairingToken: string): void {
    this.allUsernameByTab.set(tabId, { origin, pairingToken, username, createdAt: this.now() });
  }

  recall(tabId: number, origin: string, pairingToken: string): string | undefined {
    const remembered = this.allUsernameByTab.get(tabId);
    if (!remembered || remembered.origin !== origin || remembered.pairingToken !== pairingToken) return undefined;
    if (this.now() - remembered.createdAt > this.lifetimeMilliseconds) {
      this.allUsernameByTab.delete(tabId);
      return undefined;
    }
    return remembered.username;
  }

  removeTab(tabId: number): void {
    this.allUsernameByTab.delete(tabId);
  }
}
