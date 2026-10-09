import type { ErrorCode, GenerateOptions, LoginSummary } from "./protocol";

export type PopupView = "not-paired" | "desktop-unavailable" | "signed-out" | "locked" | "unlocked";

export interface PopupState {
  view: PopupView;
  email?: string;
  isOffline?: boolean;
  errorMessage?: string;
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
}

export interface FrameProbe {
  hasLoginForm: boolean;
  hasUsernameField: boolean;
  hasFocusedField: boolean;
  hasFocusedLoginField: boolean;
}

export type PopupMessage =
  | { type: "popup.getState"; shouldReconnect?: boolean }
  | { type: "popup.startPairing" }
  | { type: "popup.finishPairing"; code: string }
  | { type: "popup.unlock" }
  | { type: "popup.matchLogins"; tabId: number }
  | { type: "popup.fillLogin"; tabId: number; vaultID: string; itemID: string }
  | { type: "popup.copyPassword"; tabId: number; vaultID: string; itemID: string }
  | { type: "popup.copyText"; text: string }
  | { type: "popup.generatePassword"; options: GenerateOptions }
  | { type: "popup.fillPassword"; tabId: number; password: string };

export type ContentToBackgroundMessage =
  | { type: "content.loginSubmitted"; username: string; password: string }
  | { type: "content.usernameSubmitted"; username: string }
  | { type: "content.getPendingSaveOffer" }
  | { type: "content.actOnSaveOffer"; offerID: string; action: SaveOfferAction };

export type BackgroundToContentMessage =
  | { type: "content.probe" }
  | { type: "content.fillLogin"; expectedOrigin: string; username: string; password: string }
  | { type: "content.fillPassword"; expectedOrigin: string; password: string }
  | { type: "content.showSaveOffer"; offer: SaveOfferView };

export interface OffscreenCopyMessage {
  target: "offscreen";
  type: "offscreen.copy";
  text: string;
  clearAfterMilliseconds: number;
}

export type MessageResponse<Result> = { ok: true; result: Result } | { ok: false; error: { code: ErrorCode; message: string } };
