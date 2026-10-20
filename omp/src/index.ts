import type { ExtensionAPI, ExtensionContext } from "@oh-my-pi/pi-coding-agent";
import { classifyBashCommand, listHisarInvocations } from "./bash";
import { describeNote, findHisar, type HisarLocation, prependToPath } from "./locate";
import { type HisarStatus, probeStatus, statusTexts } from "./status";

const hisarBundleID = "com.tryhisar.hisar";

export default function hisar(pi: ExtensionAPI) {
  let location: HisarLocation | undefined;
  let probeCount = 0;

  // refreshStatus never throws, so callers can leave it running; only the newest probe sets the status.
  async function refreshStatus(ctx: ExtensionContext): Promise<HisarStatus> {
    const probe = ++probeCount;
    const status = await probeStatus(location?.path, process.env);
    if (probe === probeCount) ctx.ui.setStatus("hisar", statusTexts[status]);
    return status;
  }

  pi.on("session_start", (_event, ctx) => {
    location = findHisar(process.env);
    if (location) process.env.PATH = prependToPath(process.env.PATH, location);
    void refreshStatus(ctx);
  });

  pi.on("before_agent_start", (event) => {
    if (!location) return;
    const note = describeNote(location);
    if (event.systemPrompt.includes(note)) return;
    return { systemPrompt: [...event.systemPrompt, note] };
  });

  pi.on("tool_call", (event) => {
    if (event.toolName !== "bash") return;
    const verdict = classifyBashCommand(String(event.input.command ?? ""));
    if (verdict.block) return { block: true, reason: verdict.reason };
  });

  pi.on("tool_result", (event, ctx) => {
    if (event.toolName !== "bash") return;
    if (listHisarInvocations(String(event.input.command ?? "")).length > 0) void refreshStatus(ctx);
  });

  pi.registerCommand("hisar", {
    description: "Show Hisar's state and open Hisar to choose the vaults coding agents can use",
    handler: async (_args, ctx) => {
      const status = await refreshStatus(ctx);
      const settings = "Turn on vaults for coding agents in Hisar's Settings > Coding agents.";
      if (status === "not-installed" || process.platform !== "darwin") {
        ctx.ui.notify(`${statusTexts[status]}. ${settings}`, "info");
        return;
      }
      const opened = await pi.exec("open", ["-b", hisarBundleID]);
      if (opened.code !== 0) {
        ctx.ui.notify(`${statusTexts[status]}. Hisar did not open: ${opened.stderr.trim()}`, "error");
        return;
      }
      ctx.ui.notify(`${statusTexts[status]}. ${settings}`, "info");
    },
  });
}
