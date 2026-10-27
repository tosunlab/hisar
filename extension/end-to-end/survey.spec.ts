import { appendFileSync } from "node:fs";
import { test, type Page } from "@playwright/test";
import { launchHarness, type Harness } from "./harness";

// The report, one line per visible input, with ICON where the icon showed and the test ids of the menu that opened.
const reportFile = process.env.HISAR_SURVEY_REPORT ?? "survey.txt";

const allUrl = [
  "https://www.google.com/",
  "https://github.com/",
  "https://github.com/login",
  "https://www.youtube.com/",
  "https://www.amazon.com/",
  "https://www.reddit.com/",
  "https://www.reddit.com/login/",
  "https://x.com/i/flow/login",
  "https://www.linkedin.com/",
  "https://www.instagram.com/",
  "https://www.facebook.com/",
  "https://www.netflix.com/login",
  "https://www.trendyol.com/",
  "https://www.hepsiburada.com/",
  "https://www.sahibinden.com/",
  "https://www.n11.com/",
  "https://eksisozluk.com/",
  "https://www.booking.com/",
  "https://www.airbnb.com/",
  "https://stackoverflow.com/",
  "https://en.wikipedia.org/wiki/Main_Page",
  "https://duckduckgo.com/",
  "https://www.bing.com/",
  "https://login.microsoftonline.com/",
  "https://accounts.google.com/",
  "https://www.notion.so/login",
  "https://www.figma.com/login",
  "https://discord.com/login",
  "https://chatgpt.com/",
  "https://claude.ai/login",
  "https://www.ebay.com/",
  "https://www.etsy.com/",
  "https://www.bbc.com/",
  "https://www.nytimes.com/",
  "https://www.dropbox.com/login",
  "https://mail.proton.me/login",
  "https://www.paypal.com/signin",
  "https://news.ycombinator.com/",
  "https://www.hurriyet.com.tr/",
  "https://www.yemeksepeti.com/",
];

interface InputSummary {
  index: number;
  description: string;
}

async function listInputs(page: Page): Promise<InputSummary[]> {
  return page.evaluate(() => {
    const allSkip = new Set(["hidden", "submit", "checkbox", "radio", "button", "image", "file", "range", "color"]);
    const allInput = Array.from(document.querySelectorAll("input")).filter((input) => !allSkip.has(input.type));
    const allSummary: { index: number; description: string }[] = [];
    allInput.forEach((input, index) => {
      const rect = input.getBoundingClientRect();
      const style = getComputedStyle(input);
      if (rect.width < 2 || rect.height < 2 || style.display === "none" || style.visibility === "hidden") return;
      input.setAttribute("data-survey-index", String(index));
      const allPart = [
        `type=${input.getAttribute("type") ?? ""}`,
        input.name ? `name=${input.name}` : "",
        input.id ? `id=${input.id}` : "",
        input.getAttribute("placeholder") ? `ph="${input.getAttribute("placeholder")}"` : "",
        input.getAttribute("aria-label") ? `aria="${input.getAttribute("aria-label")}"` : "",
        input.getAttribute("autocomplete") ? `ac=${input.getAttribute("autocomplete")}` : "",
        input.form ? "form" : "formless",
        `${Math.round(rect.width)}x${Math.round(rect.height)}`,
      ];
      allSummary.push({ index, description: allPart.filter(Boolean).join(" ").slice(0, 170) });
    });
    return allSummary.slice(0, 15);
  });
}

let harness: Harness;

test.beforeAll(async () => {
  harness = await launchHarness();
  await harness.pairWithoutPopup();
});

test.afterAll(async () => {
  await harness?.close();
});

for (const url of allUrl) {
  test(`survey ${url}`, async () => {
    const page = await harness.context.newPage();
    const allLine: string[] = [];
    try {
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 30_000 }).catch((error) => allLine.push(`  goto: ${(error as Error).message.split("\n")[0]}`));
      await page.waitForTimeout(4000);
      const allInput = await listInputs(page);
      for (const input of allInput) {
        await page.evaluate((index) => {
          const element = document.querySelector<HTMLInputElement>(`[data-survey-index="${index}"]`);
          element?.focus();
        }, input.index);
        await page.waitForTimeout(400);
        const hasIcon = await page.evaluate(() => document.querySelector("hisar-inline-menu") !== null);
        let menu = "";
        if (hasIcon) {
          await page.waitForTimeout(800);
          const frame = harness.findInlineMenuFrame(page);
          const allTestId = frame ? await frame.locator("[data-testid]").evaluateAll((allElement) => allElement.map((element) => element.getAttribute("data-testid"))).catch(() => []) : [];
          menu = [...new Set(allTestId)].join(",");
        }
        allLine.push(`  ${hasIcon ? "ICON" : "    "} ${menu.padEnd(22)} ${input.description}`);
        await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
        await page.waitForTimeout(100);
      }
      allLine.unshift(`${url} (${page.url()}): ${allInput.length} visible inputs`);
    } catch (error) {
      allLine.unshift(`${url}: failed ${(error as Error).message.split("\n")[0]}`);
    } finally {
      appendFileSync(reportFile, `${allLine.join("\n")}\n`);
      await page.close();
    }
  });
}
