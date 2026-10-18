import { readFileSync } from "node:fs";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { allAppErrorCode } from "./errors";
import { answerHandshake, createSocketDirectory, startFakeApp, type FakeApp, type SocketDirectory } from "./fake-app";
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

  it("sends the contract's requests and reads its results through Hisar's methods", async () => {
    const resultByID = new Map(golden.exchanges.map((exchange) => [exchange.request.id, exchange.result]));
    app = await startFakeApp(directory.socketPath, (request) => ({
      id: request.id,
      ok: true,
      result: resultByID.get(request.id),
    }));
    const hisar = new Hisar({ socketPath: directory.socketPath, clientVersion: "1.0.0" });

    expect(await hisar.isApproved()).toBe(true);
    const itemList = await hisar.listItems();
    const code = await hisar.getCode(github);
    const password = await hisar.readPassword(github);
    const receipt = await hisar.copy(github, "password");
    const shaped = await hisar.generatePassword({ length: 20, digits: true, symbols: false });
    const generated = await hisar.generatePassword();
    await hisar.lock();
    hisar.close();

    expect(app.allRequest).toEqual(golden.exchanges.map((exchange) => exchange.request));
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
  });

  it("turns the contract's error answers into errors with their code and the app's sentence", async () => {
    let errorIndex = 0;
    app = await startFakeApp(
      directory.socketPath,
      answerHandshake((request) => ({ ...golden.errors[errorIndex++], id: request.id })),
    );
    const hisar = new Hisar({ socketPath: directory.socketPath, clientVersion: "1.0.0" });

    const denied = await hisar.listItems().catch((error: unknown) => error);
    const missing = await hisar.readPassword({ vaultID: "v-work", itemID: "i-wifi" }).catch((error: unknown) => error);
    hisar.close();

    expect(denied).toBeInstanceOf(HisarError);
    expect(denied).toMatchObject({ code: "denied", message: "Hisar denied the request (it was declined)" });
    expect(missing).toMatchObject({ code: "not-found", message: "“Office Wi-Fi” has no password" });
  });
});
