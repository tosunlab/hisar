import { describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";

const openURL = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock("./api", () => ({ api: { openURL } }));

const { openExternalLink } = await import("./externalLink");
const SignInView = (await import("./components/SignInView.vue")).default;

function clickLink(html: string): MouseEvent {
  document.body.innerHTML = html;
  const event = new MouseEvent("click", { bubbles: true, cancelable: true });
  document.body.addEventListener("click", openExternalLink, { once: true });
  document.querySelector("span, a")?.dispatchEvent(event);
  return event;
}

describe("openExternalLink", () => {
  it("opens http(s) links with target _blank in the default browser", () => {
    openURL.mockClear();
    const event = clickLink('<a href="https://api.tryhisar.com/terms" target="_blank"><span>Terms</span></a>');
    expect(event.defaultPrevented).toBe(true);
    expect(openURL).toHaveBeenCalledWith("https://api.tryhisar.com/terms");
  });

  it("leaves other links and schemes alone", () => {
    openURL.mockClear();
    expect(clickLink('<a href="#section">Section</a>').defaultPrevented).toBe(false);
    expect(clickLink('<a href="file:///etc/passwd" target="_blank">File</a>').defaultPrevented).toBe(false);
    expect(openURL).not.toHaveBeenCalled();
  });

  it("links the Terms and the Privacy Policy when creating an account", async () => {
    openURL.mockClear();
    const wrapper = mount(SignInView, { attachTo: document.body });
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    const legalLinks = wrapper.find('[data-testid="legal-links"]');
    expect(legalLinks.text()).toBe("By creating an account you agree to the Terms and Privacy Policy.");
    expect(legalLinks.findAll("a").map((link) => link.attributes("href"))).toEqual(["https://api.tryhisar.com/terms", "https://api.tryhisar.com/privacy"]);
    document.addEventListener("click", openExternalLink);
    await legalLinks.findAll("a")[1].trigger("click");
    document.removeEventListener("click", openExternalLink);
    expect(openURL).toHaveBeenCalledWith("https://api.tryhisar.com/privacy");
    wrapper.unmount();
  });
});
