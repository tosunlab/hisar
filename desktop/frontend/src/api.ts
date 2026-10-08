import { Service } from "../bindings/github.com/tryhisar/hisar/desktop/vault";
import type {
  ItemDetail as BoundItemDetail,
  ItemField,
  PasswordConfig,
  SignUpResult,
  Status,
  VaultSummary,
} from "../bindings/github.com/tryhisar/hisar/desktop/vault";

export type { ItemField, PasswordConfig, SignUpResult, Status, VaultSummary };

// ItemDetail mirrors the Go type with nil slices normalized to empty arrays.
export interface ItemDetail extends Omit<BoundItemDetail, "urls" | "fields"> {
  urls: string[];
  fields: ItemField[];
}

function normalizeItem(boundItem: BoundItemDetail): ItemDetail {
  return { ...boundItem, urls: boundItem.urls ?? [], fields: boundItem.fields ?? [] };
}

export const api = {
  getStatus: (): Promise<Status> => Service.GetStatus(),
  signIn: (server: string, email: string, secretKey: string, password: string): Promise<Status> =>
    Service.SignIn(server, email, secretKey, password),
  signUp: (inviteLink: string, email: string, name: string, password: string): Promise<SignUpResult> =>
    Service.SignUp(inviteLink, email, name, password),
  unlock: (password: string): Promise<Status> => Service.Unlock(password),
  lock: (): Promise<Status> => Service.Lock(),
  signOut: (): Promise<Status> => Service.SignOut(),
  reportActivity: (): Promise<void> => Service.ReportActivity(),
  listAllVault: async (): Promise<VaultSummary[]> => (await Service.ListAllVault()) ?? [],
  createVault: (name: string): Promise<VaultSummary> => Service.CreateVault(name),
  listAllItem: async (vaultID: string): Promise<ItemDetail[]> => ((await Service.ListAllItem(vaultID)) ?? []).map(normalizeItem),
  getItem: async (vaultID: string, itemID: string): Promise<ItemDetail> => normalizeItem(await Service.GetItem(vaultID, itemID)),
  saveItem: async (vaultID: string, detail: ItemDetail): Promise<ItemDetail> =>
    normalizeItem(await Service.SaveItem(vaultID, detail)),
  deleteItem: (vaultID: string, itemID: string): Promise<void> => Service.DeleteItem(vaultID, itemID),
  generatePassword: (config: PasswordConfig): Promise<string> => Service.GeneratePassword(config),
  copyField: (vaultID: string, itemID: string, fieldName: string): Promise<void> =>
    Service.CopyField(vaultID, itemID, fieldName),
  copyText: (text: string): Promise<void> => Service.CopyText(text),
};

export function getErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  if (typeof error === "string" && error) {
    return error;
  }
  return "Something went wrong";
}

export function createEmptyItem(): ItemDetail {
  return { id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] };
}
