import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { chromium, expect, test, type BrowserContext, type Page, type Worker } from "@playwright/test";
import { createInitialState, readState, writeState } from "../tools/fake-native-host.mjs";
import { extensionID, registerFakeHost } from "../tools/register-fake-host.mjs";
import { startTestSite, type TestSite } from "./test-site";

const distDirectory = resolve(import.meta.dirname, "../dist");
const isHeaded = process.env.HISAR_HEADED === "1";

let temporaryDirectory: string;
let stateFile: string;
let site: TestSite;
let context: BrowserContext;
let serviceWorker: Worker;

interface CdpNode {
  nodeId: number;
  backendNodeId: number;
  nodeName: string;
  nodeValue?: string;
  children?: CdpNode[];
  shadowRoots?: CdpNode[];
  contentDocument?: CdpNode;
}

function collectText(node: CdpNode): string {
  const allChild = [...(node.shadowRoots ?? []), ...(node.children ?? [])];
  return (node.nodeValue ?? "") + allChild.map(collectText).join("");
}

function findAllNode(node: CdpNode, predicate: (candidate: CdpNode) => boolean, allMatch: CdpNode[] = []): CdpNode[] {
  if (predicate(node)) allMatch.push(node);
  for (const child of [...(node.shadowRoots ?? []), ...(node.children ?? [])]) findAllNode(child, predicate, allMatch);
  return allMatch;
}

// The save bar lives in a closed shadow root, which only the DevTools protocol can see into.
async function readSaveBar(page: Page): Promise<{ text: string; clickButton: (label: string) => Promise<void> } | undefined> {
  const session = await context.newCDPSession(page);
  try {
    const { root } = (await session.send("DOM.getDocument", { depth: -1, pierce: true })) as { root: CdpNode };
    const [host] = findAllNode(root, (node) => node.nodeName === "HISAR-SAVE-BAR");
    if (!host) return undefined;
    const allButton = findAllNode(host, (node) => node.nodeName === "BUTTON");
    return {
      text: collectText(host),
      clickButton: async (label) => {
        const button = allButton.find((candidate) => collectText(candidate).trim() === label);
        if (!button) throw new Error(`no ${label} button in the save bar`);
        const clickSession = await context.newCDPSession(page);
        const { model } = (await clickSession.send("DOM.getBoxModel", { backendNodeId: button.backendNodeId })) as {
          model: { content: number[] };
        };
        const [left, top, , , right, bottom] = model.content;
        await page.mouse.click((left + right) / 2, (top + bottom) / 2);
        await clickSession.detach();
      },
    };
  } finally {
    await session.detach();
  }
}

async function getTabId(url: string): Promise<number> {
  return serviceWorker.evaluate(async (targetUrl) => {
    const [tab] = await chrome.tabs.query({ url: targetUrl });
    return tab.id as number;
  }, url);
}

test.beforeAll(async () => {
  temporaryDirectory = mkdtempSync(join(tmpdir(), "hisar-end-to-end-"));
  site = await startTestSite();
  const registration = registerFakeHost({
    userDataDirectory: join(temporaryDirectory, "profile"),
    stateFile: join(temporaryDirectory, "state.json"),
    hostDirectory: join(temporaryDirectory, "host"),
    shouldResetState: true,
  });
  stateFile = registration.stateFile;
  const state = createInitialState();
  state.pairingCode = "246810";
  state.allLogin = [
    {
      vaultID: "vault-personal",
      vaultName: "Personal",
      itemID: "item-test-site",
      title: "Test site",
      username: "alex@example.com",
      password: "old password",
      url: site.origin,
    },
  ];
  writeState(stateFile, state);
  context = await chromium.launchPersistentContext(join(temporaryDirectory, "profile"), {
    channel: "chromium",
    headless: !isHeaded,
    args: [`--disable-extensions-except=${distDirectory}`, `--load-extension=${distDirectory}`],
  });
  serviceWorker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
});

test.afterAll(async () => {
  await context?.close();
  await site?.close();
  rmSync(temporaryDirectory, { recursive: true, force: true });
});

test("loads with the extension ID from the manifest key", () => {
  expect(new URL(serviceWorker.url()).host).toBe(extensionID);
});

test("pairs, fills only same-origin frames, and offers to update a changed password", async () => {
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  const tabId = await getTabId(`${site.origin}/login`);

  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${extensionID}/popup.html?tabId=${tabId}`);
  await popup.getByTestId("start-pairing").click();
  await expect(popup.getByTestId("pairing-code")).toBeVisible();
  expect(readState(stateFile).pendingPairing?.browser).toBeTruthy();
  await popup.getByTestId("pairing-code").fill("246810");
  await popup.getByTestId("finish-pairing").click();
  await expect(popup.getByTestId("login-row")).toHaveCount(1);
  await expect(popup.getByTestId("login-row")).toContainText("Test site");
  expect(readState(stateFile).allPairing).toHaveLength(1);

  await popup.getByTestId("fill").click();
  await expect(loginPage.locator("#username")).toHaveValue("alex@example.com");
  await expect(loginPage.locator("#password")).toHaveValue("old password");
  const crossOriginFrame = loginPage.frameLocator("#other-origin");
  await expect(crossOriginFrame.locator("#frame-username")).toHaveValue("");
  await expect(crossOriginFrame.locator("#frame-password")).toHaveValue("");

  await popup.getByTestId("copy-password").click();
  await expect(popup.getByTestId("notice")).toContainText("Password copied");

  await popup.getByTestId("section-generator").click();
  await expect(popup.getByTestId("generated-password")).not.toBeEmpty();
  await popup.close();

  await loginPage.bringToFront();
  await loginPage.locator("#password").fill("new password");
  await loginPage.locator("#sign-in").click();
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  await expect.poll(async () => (await readSaveBar(loginPage))?.text ?? "", { timeout: 10_000 }).toContain(
    "Update the password for Test site in Hisar?",
  );
  await (await readSaveBar(loginPage))!.clickButton("Update");
  await expect.poll(() => readState(stateFile).allLogin[0].password).toBe("new password");
  expect(readState(stateFile).allLogin).toHaveLength(1);
});

test("offers to save a new login and stores it in the desktop", async () => {
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  await loginPage.locator("#username").fill("someone-new@example.com");
  await loginPage.locator("#password").fill("brand new");
  await loginPage.locator("#password").press("Enter");
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  await expect.poll(async () => (await readSaveBar(loginPage))?.text ?? "", { timeout: 10_000 }).toContain(
    "Save this login for 127.0.0.1",
  );
  await (await readSaveBar(loginPage))!.clickButton("Save");
  await expect
    .poll(() => readState(stateFile).allLogin.find((login) => login.username === "someone-new@example.com")?.password)
    .toBe("brand new");
});

test("the offscreen document clears the clipboard when it still holds the copied value", async () => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: site.origin });
  const page = await context.newPage();
  await page.goto(`${site.origin}/login`);
  const readClipboard = () => page.evaluate(() => navigator.clipboard.readText());
  const copyThroughOffscreen = (text: string) =>
    serviceWorker.evaluate(async (copiedText) => {
      const url = chrome.runtime.getURL("offscreen.html");
      const allContext = await chrome.runtime.getContexts({ contextTypes: [chrome.runtime.ContextType.OFFSCREEN_DOCUMENT], documentUrls: [url] });
      if (allContext.length === 0) {
        await chrome.offscreen.createDocument({ url: "offscreen.html", reasons: [chrome.offscreen.Reason.CLIPBOARD], justification: "test" });
      }
      return chrome.runtime.sendMessage({ target: "offscreen", type: "offscreen.copy", text: copiedText, clearAfterMilliseconds: 1000 });
    }, text);

  expect(await copyThroughOffscreen("temporary secret")).toEqual({ ok: true });
  expect(await readClipboard()).toBe("temporary secret");
  await expect.poll(readClipboard, { timeout: 5000 }).toBe("");

  expect(await copyThroughOffscreen("second secret")).toEqual({ ok: true });
  await page.evaluate(() => navigator.clipboard.writeText("something the user copied"));
  await page.waitForTimeout(1500);
  expect(await readClipboard()).toBe("something the user copied");
});

test("Never for this site stops offers for the origin", async () => {
  const loginPage = await context.newPage();
  await loginPage.goto(`${site.origin}/login`);
  await loginPage.locator("#username").fill("third@example.com");
  await loginPage.locator("#password").fill("pw");
  await loginPage.locator("#sign-in").click();
  await expect.poll(async () => (await readSaveBar(loginPage))?.text ?? "", { timeout: 10_000 }).toContain("Save this login");
  await (await readSaveBar(loginPage))!.clickButton("Never for this site");
  await expect.poll(async () => (await readSaveBar(loginPage)) === undefined).toBe(true);
  const allNeverOrigin = await serviceWorker.evaluate(async () => (await chrome.storage.local.get(["allNeverOrigin"])).allNeverOrigin);
  expect(allNeverOrigin).toEqual([site.origin]);

  await loginPage.goto(`${site.origin}/login`);
  await loginPage.locator("#username").fill("fourth@example.com");
  await loginPage.locator("#password").fill("pw");
  await loginPage.locator("#sign-in").click();
  await expect(loginPage.locator("h1")).toHaveText("Welcome back");
  await loginPage.waitForTimeout(1500);
  expect(await readSaveBar(loginPage)).toBeUndefined();
  expect(readState(stateFile).allLogin.some((login) => login.username === "fourth@example.com")).toBe(false);
});
