import type { SaveOfferKind, SaveOfferView } from "../shared/messages";
import type { CheckResult } from "../shared/protocol";

export function decideSaveOffer(checkResult: CheckResult, isNeverOrigin: boolean): SaveOfferKind | undefined {
  if (isNeverOrigin) return undefined;
  if (checkResult.status === "new") return "save";
  if (checkResult.status === "changed" && checkResult.vaultID && checkResult.itemID) return "update";
  return undefined;
}

export interface SaveOffer {
  offerID: string;
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
  allVault?: { vaultID: string; name: string }[];
  createdAt: number;
}

export function toSaveOfferView(offer: SaveOffer): SaveOfferView {
  const view: SaveOfferView = { offerID: offer.offerID, kind: offer.kind, host: offer.host, username: offer.username, title: offer.title };
  if (offer.allVault) {
    view.defaultTitle = offer.defaultTitle;
    view.allVault = offer.allVault;
  }
  return view;
}

export class SaveOfferStore {
  private readonly allOfferByTab = new Map<number, SaveOffer>();

  constructor(
    private readonly now: () => number = () => Date.now(),
    private readonly lifetimeMilliseconds = 120000,
  ) {}

  put(offer: SaveOffer): void {
    this.allOfferByTab.set(offer.tabId, offer);
  }

  getForTab(tabId: number): SaveOffer | undefined {
    const offer = this.allOfferByTab.get(tabId);
    if (!offer) return undefined;
    if (this.now() - offer.createdAt > this.lifetimeMilliseconds) {
      this.allOfferByTab.delete(tabId);
      return undefined;
    }
    return offer;
  }

  take(tabId: number, offerID: string): SaveOffer | undefined {
    const offer = this.getForTab(tabId);
    if (!offer || offer.offerID !== offerID) return undefined;
    this.allOfferByTab.delete(tabId);
    return offer;
  }

  removeTab(tabId: number): void {
    this.allOfferByTab.delete(tabId);
  }
}

interface RememberedUsername {
  origin: string;
  username: string;
  createdAt: number;
}

export class RememberedUsernameStore {
  private readonly allUsernameByTab = new Map<number, RememberedUsername>();

  constructor(
    private readonly now: () => number = () => Date.now(),
    private readonly lifetimeMilliseconds = 600000,
  ) {}

  remember(tabId: number, origin: string, username: string): void {
    this.allUsernameByTab.set(tabId, { origin, username, createdAt: this.now() });
  }

  recall(tabId: number, origin: string): string | undefined {
    const remembered = this.allUsernameByTab.get(tabId);
    if (!remembered || remembered.origin !== origin) return undefined;
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
