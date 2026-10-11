import type { Plan } from "../bindings/github.com/tryhisar/hisar/desktop/vault";

export const pricingURL = "https://tryhisar.com/pricing";
export const yearlyPriceLabel = "€19.99 a year";
export const monthlyPriceLabel = "€2.49 a month";

export interface PlanLimit {
  message: string;
  kind: string;
  limit: number;
  current: number;
  plan: string;
}

function parseCause(cause: unknown): unknown {
  if (typeof cause !== "string") {
    return cause;
  }
  try {
    return JSON.parse(cause);
  } catch {
    return null;
  }
}

// getPlanLimit reads the backend's PlanLimitError from a failed call's cause.
export function getPlanLimit(error: unknown): PlanLimit | null {
  if (typeof error !== "object" || error === null) {
    return null;
  }
  const cause = parseCause((error as { cause?: unknown }).cause);
  if (typeof cause !== "object" || cause === null || (cause as { code?: unknown }).code !== "plan_limit") {
    return null;
  }
  const fields = cause as Record<string, unknown>;
  return {
    message: String(fields.message ?? ""),
    kind: String(fields.kind ?? ""),
    limit: Number(fields.limit ?? 0),
    current: Number(fields.current ?? 0),
    plan: String(fields.plan ?? ""),
  };
}

export function formatPlanDate(value: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "long", year: "numeric" }).format(date);
}

function formatAllowance(count: number, maximum: number): string {
  return maximum === 0 ? `${count} of unlimited` : `${count} of ${maximum}`;
}

export function describePlanUsage(plan: Plan): string {
  return `Shared vaults: ${formatAllowance(plan.sharedVaultCount, plan.limits.maxSharedVaultCount)} · Share links: ${formatAllowance(plan.liveShareLinkCount, plan.limits.maxLiveShareLinkCount)}`;
}

// describeSubscription is one calm line about the subscription, empty when there is nothing to say.
export function describeSubscription(plan: Plan, now: Date = new Date()): string {
  const interval = plan.subscriptionInterval === "monthly" ? "Monthly" : "Yearly";
  switch (plan.subscriptionStatus) {
    case "active":
      return plan.renewsAt ? `${interval} · Renews on ${formatPlanDate(plan.renewsAt)}.` : `${interval}.`;
    case "on_trial":
      return plan.trialEndsAt ? `Trial ends on ${formatPlanDate(plan.trialEndsAt)}.` : "On trial.";
    case "past_due":
      return "The last payment didn't go through. Pro stays on while Lemon Squeezy retries.";
    case "cancelled":
      if (plan.endsAt && new Date(plan.endsAt) > now) {
        return `Pro until ${formatPlanDate(plan.endsAt)}. It won't renew.`;
      }
      return describeEndedSubscription(plan);
    case "expired":
    case "unpaid":
    case "paused":
      return describeEndedSubscription(plan);
  }
  return "";
}

function describeEndedSubscription(plan: Plan): string {
  const endedOn = plan.endsAt ? ` on ${formatPlanDate(plan.endsAt)}` : "";
  return `Your Pro plan ended${endedOn}. Everything you created stays; new shares follow Free's limits.`;
}
