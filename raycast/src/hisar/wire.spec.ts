import { readFileSync } from "node:fs";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { allAppErrorCode } from "./errors";
import { createSocketDirectory, startFakeApp, type FakeApp, type SocketDirectory } from "./fake-app";
import { Hisar, HisarError } from "./index";
import { allCopyableField } from "./model";
import { allLauncherRequestType } from "./protocol";

interface Golden {
  requestTypes: string[];
  fields: string[];
  errorCodes: string[];
  exchanges: { request: { id: string; type: string }; result: unknown }[];
  errors: { id: string; ok: false; error: { code: string; message: string } }[];
}

const golden = JSON.parse(
  readFileSync(new URL("../../../core/cliprotocol/testdata/launcher.json", import.meta.url), "utf8"),
) as Golden;

const github = { vaultID: "v-personal", itemID: "i-github" };
const wifi = { vaultID: "v-work", itemID: "i-wifi" };

describe("the launcher contract (core/cliprotocol/testdata/launcher.json)", () => {
  it("lists the same request types, copyable fields and error codes as the app", () => {
    expect([...allLauncherRequestType]).toEqual(golden.requestTypes);
    expect([...allCopyableField]).toEqual(golden.fields);
    expect([...allAppErrorCode]).toEqual(golden.errorCodes);
  });
});

describe("every exchange of the contract", () => {
  let directory: SocketDirectory;
  let app: FakeApp | undefined;

  beforeEach(() => {
    directory = createSocketDirectory();
  });

  afterEach(async () => {
    await app?.close();
    app = undefined;
    directory.remove();
  });

  it("sends the contract's requests in order on one connection and reads its answers through Hisar's methods", async () => {
    const responseByID = new Map<string, unknown>([
      ...golden.exchanges.map(({ request, result }) => [request.id, { id: request.id, ok: true, result }] as const),
      ...golden.errors.map((error) => [error.id, error] as const),
    ]);
    app = await startFakeApp(directory.socketPath, (request) => {
      const response = responseByID.get(request.id) as { result: object };
      // The contract's hello predates launcher.items.open and launcher.items.share; this app announces every type.
      if (request.type === "hello")
        return { ...response, result: { ...response.result, features: golden.requestTypes } };
      return response;
    });
    const hisar = new Hisar({ socketPath: directory.socketPath, clientVersion: "1.0.0" });

    expect(await hisar.isApproved()).toBe(true);
    const itemList = await hisar.listItems();
    const code = await hisar.getCode(github);
    const password = await hisar.readPassword(github);
    const receipt = await hisar.copy(github, "password");
    const shaped = await hisar.generatePassword({ length: 20, digits: true, symbols: false });
    const generated = await hisar.generatePassword();
    await hisar.lock();
    await hisar.open(github);
    await hisar.open(github, { isEditing: true });
    const shareLinkURL = await hisar.share(github, {
      allField: ["username", "password"],
      expiresInSeconds: 604_800,
      maxViews: 1,
    });
    const denied = await hisar.listItems().catch((error: unknown) => error);
    const missing = await hisar.readPassword(wifi).catch((error: unknown) => error);
    hisar.close();

    expect(app.allRequest).toEqual([
      ...golden.exchanges.map((exchange) => exchange.request),
      { id: golden.errors[0].id, type: "launcher.items.list" },
      { id: golden.errors[1].id, type: "launcher.items.password", ...wifi },
    ]);
    expect(itemList.allVault).toEqual([
      { id: "v-personal", name: "Personal" },
      { id: "v-work", name: "Work" },
    ]);
    expect(itemList.allItem).toEqual([
      {
        vaultID: "v-personal",
        itemID: "i-github",
        title: "GitHub",
        category: "login",
        username: "alice",
        allURL: ["https://github.com"],
        allTag: ["dev"],
        isFavorite: true,
        hasPassword: true,
        hasOneTimeCode: true,
        allReference: [
          { field: "username", reference: "hisar://Personal/GitHub/username" },
          { field: "password", reference: "hisar://Personal/GitHub/password" },
          { field: "one-time code", reference: "hisar://Personal/GitHub/otp" },
        ],
      },
      {
        vaultID: "v-work",
        itemID: "i-wifi",
        title: "Office Wi-Fi",
        category: "secure-note",
        username: "",
        allURL: [],
        allTag: [],
        isFavorite: false,
        hasPassword: false,
        hasOneTimeCode: false,
        allReference: [{ field: "notes", reference: "hisar://Work/Office%20Wi-Fi/notes" }],
      },
    ]);
    expect(code).toEqual({ code: "492039", expiresAt: new Date("2026-10-17T09:00:30Z"), period: 30 });
    expect(password).toBe("correct horse battery staple");
    expect(receipt).toEqual({ clearAfterSeconds: 90 });
    expect(shaped).toBe("Wq7tLmZ3pK9bRx2cVh8n");
    expect(generated).toBe("k4!Rz9#pW2@mXq7&Lb5$Tn8^");
    expect(shareLinkURL).toBe("https://hisar.example.com/s/Rk3vQ9xT2mLp8sWz#key");
    expect(denied).toBeInstanceOf(HisarError);
    expect(denied).toMatchObject({ code: "denied", message: "Hisar denied the request (it was declined)" });
    expect(missing).toMatchObject({ code: "not-found", message: "“Office Wi-Fi” has no password" });
  });
});
