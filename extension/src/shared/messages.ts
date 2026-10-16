import type { AddressSummary, AddressValues, ErrorCode, GenerateOptions, LoginSummary, UpdateRequirement } from "./protocol";

export type PopupView = "not-paired" | "desktop-unavailable" | "signed-out" | "locked" | "unlocked" | "update-required";

export interface PopupState {
  view: PopupView;
  email?: string;
  isOffline?: boolean;
  errorMessage?: string;
  updateRequired?: UpdateRequirement;
  // isAddressFillAvailable: the app fills addresses, so the popup offers its Addresses tab.
  isAddressFillAvailable?: boolean;
}

export interface MatchResult {
  url: string;
  allLogin: LoginSummary[];
}

export type SaveOfferKind = "save" | "update";
export type SaveOfferAction = "save" | "not-now" | "never";

export interface SaveOfferView {
  offerID: string;
  kind: SaveOfferKind;
  host: string;
  username: string;
  title?: string;
}

export interface FillOutcome {
  filledUsername: boolean;
  filledPassword: boolean;
  filledOneTimeCode?: boolean;
}

export interface FrameProbe {
  hasLoginForm: boolean;
  hasUsernameField: boolean;
  hasFocusedField: boolean;
  hasFocusedLoginField: boolean;
  hasOneTimeCodeField?: boolean;
  hasAddressForm?: boolean;
  hasFocusedAddressField?: boolean;
}

export type InlineMenuPurpose = "login" | "new-password" | "one-time-code" | "address";
export type InlineMenuKey = "ArrowDown" | "ArrowUp" | "Enter";
export type InlineMenuView = PopupView | "logins";

export interface InlineMenuState {
  view: InlineMenuView;
  purpose: InlineMenuPurpose;
  host: string;
  allLogin: LoginSummary[];
  hasMoreLogin: boolean;
  // allAddress holds the addresses offered on an address field, in place of logins.
  allAddress?: AddressSummary[];
  hasMoreAddress?: boolean;
  suggestedPassword?: string;
  errorMessage?: string;
  updateRequired?: UpdateRequirement;
}

export type PopupMessage =
  | { type: "popup.getState"; shouldReconnect?: boolean }
  | { type: "popup.startPairing" }
  | { type: "popup.finishPairing"; code: string }
  | { type: "popup.unlock" }
  | { type: "popup.matchLogins"; tabId: number }
  | { type: "popup.fillLogin"; tabId: number; vaultID: string; itemID: string }
  | { type: "popup.getPassword"; tabId: number; vaultID: string; itemID: string }
  | { type: "popup.getOneTimeCode"; tabId: number; vaultID: string; itemID: string }
  | { type: "popup.generatePassword"; options: GenerateOptions }
  | { type: "popup.fillPassword"; tabId: number; password: string }
  | { type: "popup.listAddresses" }
  | { type: "popup.fillAddress"; tabId: number; vaultID: string; itemID: string };

export type ContentToBackgroundMessage =
  | { type: "content.loginSubmitted"; username: string; password: string }
  | { type: "content.usernameSubmitted"; username: string }
  | { type: "content.getPendingSaveOffer" }
  | { type: "content.actOnSaveOffer"; offerID: string; action: SaveOfferAction }
  | { type: "content.openInlineMenu"; purpose: InlineMenuPurpose }
  | { type: "content.closeInlineMenu"; sessionID: string }
  | { type: "content.pressInlineMenuKey"; sessionID: string; key: InlineMenuKey };

export type InlineMenuMessage =
  | { type: "inlineMenu.getState"; sessionID: string }
  | { type: "inlineMenu.fillLogin"; sessionID: string; vaultID: string; itemID: string }
  | { type: "inlineMenu.fillOneTimeCode"; sessionID: string; vaultID: string; itemID: string }
  | { type: "inlineMenu.fillAddress"; sessionID: string; vaultID: string; itemID: string }
  | { type: "inlineMenu.useSuggestedPassword"; sessionID: string }
  | { type: "inlineMenu.unlock"; sessionID: string }
  | { type: "inlineMenu.openHisar"; sessionID: string }
  | { type: "inlineMenu.openPopup"; sessionID: string }
  | { type: "inlineMenu.resize"; sessionID: string; height: number }
  | { type: "inlineMenu.close"; sessionID: string };

export interface InlineMenuBroadcast {
  target: "inline-menu";
  type: "inlineMenu.pressKey";
  sessionID: string;
  key: InlineMenuKey;
}

export type BackgroundToContentMessage =
  | { type: "content.probe" }
  | { type: "content.fillLogin"; expectedOrigin: string; username: string; password: string; sessionID?: string }
  | { type: "content.fillPassword"; expectedOrigin: string; password: string; sessionID?: string }
  | { type: "content.fillOneTimeCode"; expectedOrigin: string; code: string; sessionID?: string }
  | { type: "content.fillAddress"; expectedOrigin: string; address: AddressValues; sessionID?: string }
  | { type: "content.showSaveOffer"; offer: SaveOfferView }
  | { type: "content.isInlineMenuVisible"; sessionID: string }
  | { type: "content.resizeInlineMenu"; sessionID: string; height: number }
  | { type: "content.dismissInlineMenu"; sessionID: string };

export interface OffscreenCopyMessage {
  target: "offscreen";
  type: "offscreen.copy";
  text: string;
}

export type MessageResponse<Result> = { ok: true; result: Result } | { ok: false; error: { code: ErrorCode; message: string } };
