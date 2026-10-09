import type { FrameProbe } from "../shared/messages";
import { isWebUrl } from "../shared/url";

export interface FrameInfo {
  frameId: number;
  url: string;
  origin: string;
  allAncestorOrigin: string[];
}

export interface ProbedFrame extends FrameInfo {
  probe: FrameProbe;
}

export function selectAllFillableFrame(allFrame: FrameInfo[]): FrameInfo[] {
  const topFrame = allFrame.find((frame) => frame.frameId === 0);
  if (!topFrame || !isWebUrl(topFrame.url) || topFrame.origin === "null") return [];
  return allFrame.filter(
    (frame) =>
      frame.origin === topFrame.origin && frame.allAncestorOrigin.every((origin) => origin === topFrame.origin),
  );
}

export function chooseLoginFrame(allProbed: ProbedFrame[]): ProbedFrame | undefined {
  return (
    allProbed.find((frame) => frame.probe.hasFocusedLoginField) ??
    allProbed.find((frame) => frame.frameId === 0 && frame.probe.hasLoginForm) ??
    allProbed.find((frame) => frame.probe.hasLoginForm) ??
    allProbed.find((frame) => frame.probe.hasUsernameField)
  );
}

export function chooseFocusedFrame(allProbed: ProbedFrame[]): ProbedFrame | undefined {
  return allProbed.find((frame) => frame.probe.hasFocusedField);
}

export function chooseOneTimeCodeFrame(allProbed: ProbedFrame[]): ProbedFrame | undefined {
  return allProbed.find((frame) => frame.frameId === 0 && frame.probe.hasOneTimeCodeField) ?? allProbed.find((frame) => frame.probe.hasOneTimeCodeField);
}
