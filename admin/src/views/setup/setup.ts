import { request, type InstanceCheck } from "../../api";

export type SetupState = "needsAdmin" | "needsChecks" | "needsInvitePolicy" | "done";

export type SetupStep = "account" | "emergencyKit" | "checks" | "invitePolicy";

export type InvitePolicy = "anyone" | "admins";

export interface Setup {
  state: SetupState;
}

export const setupSteps: readonly { step: SetupStep; title: string }[] = [
  { step: "account", title: "Create the first admin" },
  { step: "emergencyKit", title: "Save your Emergency Kit" },
  { step: "checks", title: "Check the server" },
  { step: "invitePolicy", title: "Choose who can invite people" },
];

export const invalidSetupLinkMessage = "This setup link is no longer valid. Use the link in the latest server log.";

export const emergencyKitFileName = "Hisar Emergency Kit.pdf";

// findStep is the step a page opened on the server's setup state starts at; null once setup is done.
export function findStep(state: SetupState): SetupStep | null {
  switch (state) {
    case "needsAdmin":
      return "account";
    case "needsChecks":
      return "checks";
    case "needsInvitePolicy":
      return "invitePolicy";
    case "done":
      return null;
  }
}

export function getSetup(): Promise<Setup> {
  return request("GET", "/setup");
}

export async function listSetupChecks(): Promise<InstanceCheck[]> {
  return (await request<{ checks: InstanceCheck[] }>("GET", "/admin/setup/checks")).checks;
}

export function finishChecksStep(): Promise<Setup> {
  return request("POST", "/admin/setup/checks");
}

export function finishInvitePolicyStep(invitePolicy: InvitePolicy): Promise<Setup> {
  return request("POST", "/admin/setup/invite-policy", { body: { invitePolicy } });
}

export function downloadFile(content: Uint8Array<ArrayBuffer>, fileName: string, type: string): void {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
