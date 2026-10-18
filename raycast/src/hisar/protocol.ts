import { HisarError, isAppErrorCode } from "./errors";
import type { CopyReceipt, CopyableField, Item, ItemList, ItemReference, OneTimeCode, PasswordOptions } from "./model";

export const protocolVersion = 1;
export const clientPlatform = "darwin/raycast";

/** Must equal cliprotocol.ListLauncherRequestTypes; hello announces each one as a feature. */
export const allLauncherRequestType = [
  "launcher.hello",
  "launcher.items.list",
  "launcher.items.code",
  "launcher.items.password",
  "launcher.items.copy",
  "launcher.passwords.generate",
  "launcher.lock",
] as const satisfies readonly RequestType[];

export type LauncherRequestType = (typeof allLauncherRequestType)[number];

export interface Hello {
  protocolVersion: number;
  allFeature: string[];
}

type NoParameters = Record<string, never>;

/** RequestMap names each request's wire parameters and the domain value its result parses into. */
export interface RequestMap {
  hello: { parameters: { clientPlatform: string; clientVersion: string }; result: Hello };
  "launcher.hello": { parameters: NoParameters; result: { isApproved: boolean } };
  "launcher.items.list": { parameters: NoParameters; result: ItemList };
  "launcher.items.code": { parameters: ItemReference; result: OneTimeCode };
  "launcher.items.password": { parameters: ItemReference; result: string };
  "launcher.items.copy": { parameters: ItemReference & { field: CopyableField }; result: CopyReceipt };
  "launcher.passwords.generate": { parameters: { generator?: PasswordOptions }; result: string };
  "launcher.lock": { parameters: NoParameters; result: void };
}

export type RequestType = keyof RequestMap;

export type Response = { id: string; result: unknown } | { id: string; error: HisarError };

type WireObject = Record<string, unknown>;

/** parseResponse reads one answer; an error answer carries a HisarError, and a code this client does not know is internal. */
export function parseResponse(message: unknown): Response {
  const response = readObject(message, "the response");
  const id = readString(response, "id");
  if (response.ok === true) return { id, result: response.result };
  const error = readObject(response.error, "the error");
  const code = error.code;
  const text = typeof error.message === "string" && error.message !== "" ? error.message : String(code);
  return { id, error: new HisarError(isAppErrorCode(code) ? code : "internal", text) };
}

/** parseResultByType turns each request's result into domain values; nothing else reads wire results. */
export const parseResultByType: { [Type in RequestType]: (result: unknown) => RequestMap[Type]["result"] } = {
  hello: (result) => {
    const hello = readObject(result, "hello");
    return { protocolVersion: readNumber(hello, "protocolVersion"), allFeature: readStringList(hello, "features") };
  },
  "launcher.hello": (result) => ({ isApproved: readBoolean(readObject(result, "launcher.hello"), "isApproved") }),
  "launcher.items.list": (result) => {
    const list = readObject(result, "the item list");
    return {
      allVault: readObjectList(list, "vaults").map((vault) => ({
        id: readString(vault, "id"),
        name: readString(vault, "name"),
      })),
      allItem: readObjectList(list, "items").map(parseItem),
    };
  },
  "launcher.items.code": (result) => {
    const code = readObject(result, "the one-time code");
    const expiresAt = new Date(readString(code, "expiresAt"));
    if (Number.isNaN(expiresAt.getTime())) failToUnderstand("expiresAt is not a time");
    return { code: readString(code, "code"), expiresAt, period: readNumber(code, "period") };
  },
  "launcher.items.password": (result) => readString(readObject(result, "the password"), "password"),
  "launcher.items.copy": (result) => ({
    clearAfterSeconds: readNumber(readObject(result, "the copy"), "clearAfterSeconds"),
  }),
  "launcher.passwords.generate": (result) => readString(readObject(result, "the generated password"), "password"),
  "launcher.lock": (result) => {
    readObject(result, "the lock");
  },
};

function parseItem(item: WireObject): Item {
  return {
    vaultID: readString(item, "vaultID"),
    itemID: readString(item, "id"),
    title: readString(item, "title"),
    category: readString(item, "category"),
    username: readString(item, "username"),
    allURL: readStringList(item, "urls"),
    allTag: readStringList(item, "tags"),
    isFavorite: readBoolean(item, "isFavorite"),
    hasPassword: readBoolean(item, "hasPassword"),
    hasOneTimeCode: readBoolean(item, "hasOneTimeCode"),
    allReference: readObjectList(item, "references").map((reference) => ({
      field: readString(reference, "field"),
      reference: readString(reference, "reference"),
    })),
  };
}

function failToUnderstand(detail: string): never {
  throw new HisarError("internal", `Hisar sent an answer this extension does not understand: ${detail}`);
}

function readObject(value: unknown, name: string): WireObject {
  if (typeof value !== "object" || value === null || Array.isArray(value)) failToUnderstand(`${name} is not an object`);
  return value as WireObject;
}

function readString(object: WireObject, key: string): string {
  const value = object[key];
  if (typeof value !== "string") failToUnderstand(`${key} is not text`);
  return value;
}

function readBoolean(object: WireObject, key: string): boolean {
  const value = object[key];
  if (typeof value !== "boolean") failToUnderstand(`${key} is not true or false`);
  return value;
}

function readNumber(object: WireObject, key: string): number {
  const value = object[key];
  if (typeof value !== "number" || !Number.isFinite(value)) failToUnderstand(`${key} is not a number`);
  return value;
}

// Go encodes an empty slice as null, and omitempty lists are missing.
function readList(object: WireObject, key: string): unknown[] {
  const value = object[key];
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) failToUnderstand(`${key} is not a list`);
  return value;
}

function readStringList(object: WireObject, key: string): string[] {
  return readList(object, key).map((entry) => {
    if (typeof entry !== "string") failToUnderstand(`${key} holds something other than text`);
    return entry;
  });
}

function readObjectList(object: WireObject, key: string): WireObject[] {
  return readList(object, key).map((entry) => readObject(entry, `an entry of ${key}`));
}
