import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createPlan, createStatus, proLimits } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventMock = vi.hoisted(() => ({ allHandlerByName: {} as Record<string, () => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({}),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, handler: () => void) => {
      eventMock.allHandlerByName[name] = handler;
      return () => delete eventMock.allHandlerByName[name];
    },
  },
}));

const { store } = await import("../store");
const { getPlanLimit, describeSubscription } = await import("../plan");
const PlanSection = (await import("./PlanSection.vue")).default;
const PlanLimitNotice = (await import("./PlanLimitNotice.vue")).default;
const api = apiMock.current;

async function mountPlanSection(): Promise<VueWrapper> {
  const wrapper = mount(PlanSection, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

function getText(wrapper: VueWrapper, testID: string): string {
  return wrapper.find(`[data-testid="${testID}"]`).text();
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("unlocked"), plan: null, isPlanUnavailable: false, planLimit: null, errorMessage: "" });
});

describe("Settings → Account plan", () => {
  it("shows Free with usage and upgrades yearly by default", async () => {
    api.getPlan.mockResolvedValue(createPlan({ sharedVaultCount: 2, liveShareLinkCount: 3 }));
    const wrapper = await mountPlanSection();
    expect(getText(wrapper, "plan-title")).toBe("Free");
    expect(getText(wrapper, "plan-usage")).toBe("Shared vaults: 2 of 2 · Share links: 3 of 10");
    expect(wrapper.find('[data-testid="manage-subscription"]').exists()).toBe(false);

    await wrapper.find('[data-testid="upgrade-to-pro"]').trigger("click");
    expect(wrapper.find('[data-testid="billing-interval-yearly"]').attributes("aria-checked")).toBe("true");
    expect(getText(wrapper, "checkout-choice")).toContain("€19.99 a year");
    await wrapper.find('[data-testid="continue-to-checkout"]').trigger("click");
    await flushPromises();
    expect(api.createCheckoutURL).toHaveBeenCalledWith("yearly");
    expect(api.openURL).toHaveBeenCalledWith("https://hisar.lemonsqueezy.com/checkout/custom/1");
    wrapper.unmount();
  });

  it("offers monthly as the other choice", async () => {
    api.getPlan.mockResolvedValue(createPlan());
    const wrapper = await mountPlanSection();
    await wrapper.find('[data-testid="upgrade-to-pro"]').trigger("click");
    await wrapper.find('[data-testid="billing-interval-monthly"]').trigger("click");
    expect(getText(wrapper, "checkout-choice")).toContain("€2.49 a month");
    await wrapper.find('[data-testid="continue-to-checkout"]').trigger("click");
    await flushPromises();
    expect(api.createCheckoutURL).toHaveBeenCalledWith("monthly");
    wrapper.unmount();
  });

  it("shows a Pro subscription's renewal and manages it in the browser", async () => {
    api.getPlan.mockResolvedValue(
      createPlan({ plan: "pro", title: "Pro", source: "subscription", subscriptionStatus: "active", subscriptionInterval: "yearly", renewsAt: "2027-10-11T09:30:00Z", limits: proLimits, sharedVaultCount: 4, isCheckoutAvailable: false, isManageAvailable: true }),
    );
    const wrapper = await mountPlanSection();
    expect(getText(wrapper, "plan-title")).toBe("Pro");
    expect(getText(wrapper, "plan-description")).toBe("Yearly · Renews on 11 October 2027.");
    expect(getText(wrapper, "plan-usage")).toBe("Shared vaults: 4 of unlimited · Share links: 3 of 200");
    expect(wrapper.find('[data-testid="upgrade-to-pro"]').exists()).toBe(false);
    await wrapper.find('[data-testid="manage-subscription"]').trigger("click");
    await flushPromises();
    expect(api.openURL).toHaveBeenCalledWith("https://hisar.lemonsqueezy.com/billing");
    wrapper.unmount();
  });

  it("describes every subscription state calmly", () => {
    const now = new Date("2026-10-11T12:00:00Z");
    const base = createPlan({ plan: "pro", title: "Pro" });
    expect(describeSubscription({ ...base, subscriptionStatus: "cancelled", endsAt: "2027-01-11T09:30:00Z" }, now)).toBe("Pro until 11 January 2027. It won't renew.");
    expect(describeSubscription({ ...base, subscriptionStatus: "past_due" }, now)).toBe("The last payment didn't go through. Pro stays on while Lemon Squeezy retries.");
    expect(describeSubscription({ ...base, subscriptionStatus: "on_trial", trialEndsAt: "2026-10-25T00:00:00Z" }, now)).toBe("Trial ends on 25 October 2026.");
    expect(describeSubscription({ ...base, subscriptionStatus: "expired", endsAt: "2026-09-30T00:00:00Z" }, now)).toBe(
      "Your Pro plan ended on 30 September 2026. Everything you created stays; new shares follow Free's limits.",
    );
    expect(describeSubscription({ ...base, subscriptionStatus: "cancelled", endsAt: "2026-09-30T00:00:00Z" }, now)).toContain("Your Pro plan ended on 30 September 2026.");
    expect(describeSubscription(createPlan(), now)).toBe("");
  });

  it("shows a lapsed plan as Free with its end and the upgrade", async () => {
    api.getPlan.mockResolvedValue(createPlan({ subscriptionStatus: "expired", subscriptionInterval: "monthly", endsAt: "2026-09-30T00:00:00Z", sharedVaultCount: 5, isManageAvailable: true }));
    const wrapper = await mountPlanSection();
    expect(getText(wrapper, "plan-title")).toBe("Free");
    expect(getText(wrapper, "plan-description")).toContain("Everything you created stays");
    expect(getText(wrapper, "plan-usage")).toContain("Shared vaults: 5 of 2");
    wrapper.unmount();
  });

  it("shows the Founding member badge without Upgrade or Manage", async () => {
    api.getPlan.mockResolvedValue(createPlan({ plan: "pro", title: "Pro", source: "founding", isFoundingMember: true, limits: proLimits, isCheckoutAvailable: false }));
    const wrapper = await mountPlanSection();
    expect(getText(wrapper, "founding-member-badge")).toBe("Founding member");
    expect(getText(wrapper, "plan-description")).toContain("Pro, free for life");
    expect(wrapper.find('[data-testid="upgrade-to-pro"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="manage-subscription"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("says quietly when the plan cannot be loaded", async () => {
    api.getPlan.mockRejectedValue(new Error("cannot reach the Hisar server"));
    const wrapper = await mountPlanSection();
    expect(getText(wrapper, "plan-unavailable")).toBe("Plan details need a connection to the Hisar server.");
    wrapper.unmount();
  });

  it("reloads when the server says the plan changed", async () => {
    api.getPlan.mockResolvedValue(createPlan());
    const wrapper = await mountPlanSection();
    api.getPlan.mockResolvedValue(createPlan({ plan: "pro", title: "Pro", limits: proLimits, isCheckoutAvailable: false }));
    eventMock.allHandlerByName["plan-changed"]();
    await flushPromises();
    expect(getText(wrapper, "plan-title")).toBe("Pro");
    wrapper.unmount();
  });

  it("opens the pricing page when this server sells no plans", async () => {
    api.getPlan.mockResolvedValue(createPlan());
    api.createCheckoutURL.mockRejectedValue(new Error("plans are not sold on this server"));
    const wrapper = await mountPlanSection();
    await wrapper.find('[data-testid="upgrade-to-pro"]').trigger("click");
    await wrapper.find('[data-testid="continue-to-checkout"]').trigger("click");
    await flushPromises();
    expect(api.openURL).toHaveBeenCalledWith("https://tryhisar.com/pricing");
    wrapper.unmount();
  });
});

describe("plan limits", () => {
  const cause = { message: "Free includes 2 shared vaults. Remove one to create another.", code: "plan_limit", kind: "shared-vaults", limit: 2, current: 2, plan: "free" };

  it("reads the limit from an object or a JSON cause", () => {
    expect(getPlanLimit(Object.assign(new Error(cause.message), { cause }))).toEqual({ message: cause.message, kind: "shared-vaults", limit: 2, current: 2, plan: "free" });
    expect(getPlanLimit(Object.assign(new Error(cause.message), { cause: JSON.stringify(cause) }))?.kind).toBe("shared-vaults");
    expect(getPlanLimit(new Error("you do not have permission to do that"))).toBeNull();
    expect(getPlanLimit(Object.assign(new Error("x"), { cause: "not json" }))).toBeNull();
    expect(getPlanLimit(Object.assign(new Error("x"), { cause: { code: "other" } }))).toBeNull();
    expect(getPlanLimit("text")).toBeNull();
  });

  it("offers the upgrade once on Free and never on Pro", async () => {
    store.errorMessage = cause.message;
    store.planLimit = { message: cause.message, kind: "shared-vaults", limit: 2, current: 2, plan: "free" };
    const freeNotice = mount(PlanLimitNotice);
    expect(freeNotice.findAll('[data-testid="plan-limit-upgrade"]')).toHaveLength(1);
    expect(freeNotice.text()).toBe("Upgrade to Pro on tryhisar.com");
    freeNotice.unmount();

    store.planLimit = { ...store.planLimit, plan: "pro" };
    const proNotice = mount(PlanLimitNotice);
    expect(proNotice.find('[data-testid="plan-limit-notice"]').exists()).toBe(false);
    proNotice.unmount();
  });

  it("disappears with the error it belongs to", async () => {
    store.planLimit = { message: cause.message, kind: "shared-vaults", limit: 2, current: 2, plan: "free" };
    store.errorMessage = "cannot reach the Hisar server";
    const notice = mount(PlanLimitNotice);
    expect(notice.find('[data-testid="plan-limit-notice"]').exists()).toBe(false);
    notice.unmount();
  });

  it("keeps the limit from a refused vault so the notice can show", async () => {
    const { createVault } = await import("../store");
    api.createVault.mockRejectedValue(Object.assign(new Error(cause.message), { cause }));
    expect(await createVault("Work")).toBe(false);
    expect(store.errorMessage).toBe(cause.message);
    expect(store.planLimit?.kind).toBe("shared-vaults");
  });
});
