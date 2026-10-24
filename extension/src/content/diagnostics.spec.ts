import { afterEach, describe, expect, it, vi } from "vitest";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import { setFillDiagnosticsEnabled } from "./diagnostics";
import { fillLogin } from "./fill";

afterEach(() => {
  setFillDiagnosticsEnabled(false);
  vi.restoreAllMocks();
});

describe("fill diagnostics", () => {
  it("write nothing until turned on, then name the fields without their values", async () => {
    const debug = vi.spyOn(console, "debug").mockImplementation(() => undefined);
    document.body.innerHTML = simpleLoginPage;
    await fillLogin(document, "alex", "secret-password");
    expect(debug).not.toHaveBeenCalled();
    setFillDiagnosticsEnabled(true);
    document.body.innerHTML = simpleLoginPage;
    await fillLogin(document, "alex", "secret-password");
    const logged = JSON.stringify(debug.mock.calls);
    expect(logged).toContain("input#password type=password");
    expect(logged).toContain('"filledPassword":true');
    expect(logged).not.toContain("secret-password");
    expect(logged).not.toContain("alex");
  });
});
