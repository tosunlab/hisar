import { expect, test } from "@playwright/test";

for (const listWidth of [240, 480]) {
  for (const reminder of ["new", "3-days", "14-days"]) {
    test(`${reminder} reminder at ${listWidth}px stays in flow and leaves controls reachable`, async ({ page }) => {
      await page.setViewportSize({ width: listWidth === 240 ? 900 : 1200, height: 480 });
      await page.addInitScript((list) => localStorage.setItem("hisar.mainWindow.columnWidths", JSON.stringify({ sidebar: 180, list })), listWidth);
      const email = `${"a".repeat(52)}@example.com`;
      await page.goto(`/?emergencyKit=${reminder}&email=${email}&appearance=light`);
      const banner = page.getByTestId("emergency-kit-banner");
      await expect(banner).toBeVisible();
      await expect(page.locator('section.list [role="option"]').first()).toBeAttached();
      await expect(page.locator(".sidebar").getByTestId("emergency-kit-banner")).toHaveCount(0);
      const layout = await page.evaluate(() => {
        const banner = document.querySelector<HTMLElement>('[data-testid="emergency-kit-banner"]')!;
        const list = document.querySelector<HTMLElement>("section.list")!;
        const rect = banner.getBoundingClientRect();
        const listRect = list.getBoundingClientRect();
        const intersects = (other: DOMRect) => rect.left < other.right && rect.right > other.left && rect.top < other.bottom && rect.bottom > other.top;
        const failures: string[] = [];
        let checked = 0;
        for (const control of document.querySelectorAll<HTMLElement>("button, a, input, select, [tabindex]")) {
          if (banner.contains(control) || !control.checkVisibility() || control.closest('[inert]')) continue;
          const box = control.getBoundingClientRect();
          if (!box.width || !box.height) continue;
          const x = box.left + box.width / 2;
          const y = box.top + box.height / 2;
          if (x < 0 || x >= innerWidth || y < 0 || y >= innerHeight) continue;
          let clipped = false;
          for (let parent = control.parentElement; parent; parent = parent.parentElement) {
            const style = getComputedStyle(parent);
            const bounds = parent.getBoundingClientRect();
            if ((style.overflowX !== "visible" && (x < bounds.left || x >= bounds.right)) || (style.overflowY !== "visible" && (y < bounds.top || y >= bounds.bottom))) clipped = true;
          }
          if (clipped) continue;
          checked++;
          if (intersects(box)) failures.push(`Banner intersects ${control.outerHTML.slice(0, 120)}`);
          const target = document.elementFromPoint(x, y);
          if (target !== control && !control.contains(target)) failures.push(`Control is covered: ${control.outerHTML.slice(0, 120)}`);
        }
        return {
          failures, checked,
          inside: rect.left >= listRect.left && rect.right <= listRect.right && rect.top >= listRect.top && rect.bottom <= listRect.bottom,
          accountOverlap: intersects(document.querySelector(".account")!.getBoundingClientRect()),
          listWidth: listRect.width,
          horizontalOverflow: banner.scrollWidth > banner.clientWidth,
        };
      });
      expect(layout.listWidth).toBe(listWidth);
      expect(layout.inside).toBe(true);
      expect(layout.accountOverlap).toBe(false);
      expect(layout.horizontalOverflow).toBe(false);
      expect(layout.checked).toBeGreaterThan(5);
      expect(layout.failures).toEqual([]);
      for (const button of await banner.locator("button").all()) await button.click({ trial: true });
      await banner.getByRole("button", { name: "Hide until Hisar locks" }).click();
      await expect(banner).toHaveCount(0);
    });
  }
}
