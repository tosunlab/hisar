import { describe, expect, test } from "bun:test";
import { describePathNote, findHisar, prependToPath } from "../src/locate";

const appHelper = "/Applications/Hisar.app/Contents/Helpers/hisar";
const homeHelper = "/Users/a/Applications/Hisar.app/Contents/Helpers/hisar";

function existing(...paths: string[]) {
  return (path: string) => paths.includes(path);
}

describe("findHisar", () => {
  const environment = { PATH: "/usr/bin:/opt/homebrew/bin", HOME: "/Users/a" };

  test("finds hisar on PATH first, in PATH order", () => {
    expect(findHisar(environment, existing("/opt/homebrew/bin/hisar", "/usr/bin/hisar", appHelper))).toEqual({
      path: "/usr/bin/hisar",
      isOnInheritedPath: true,
    });
  });

  test("then in /Applications, then in ~/Applications", () => {
    expect(findHisar(environment, existing(appHelper, homeHelper))).toEqual({
      path: appHelper,
      isOnInheritedPath: false,
    });
    expect(findHisar(environment, existing(homeHelper))).toEqual({ path: homeHelper, isOnInheritedPath: false });
  });

  test("then where a Linux AppImage or Flatpak copied it", () => {
    const copied = "/Users/a/.local/share/hisar/bin/hisar";
    expect(findHisar(environment, existing(copied))).toEqual({ path: copied, isOnInheritedPath: false });
    expect(findHisar(environment, existing(homeHelper, copied))).toEqual({
      path: homeHelper,
      isOnInheritedPath: false,
    });
  });

  test("finds nothing when Hisar is not installed", () => {
    expect(findHisar(environment, existing())).toBeUndefined();
    expect(findHisar({}, existing(homeHelper))).toBeUndefined();
  });
});

describe("prependToPath", () => {
  test("puts the app's helper folder first, once", () => {
    const location = { path: appHelper, isOnInheritedPath: false };
    const path = prependToPath("/usr/bin:/bin", location);
    expect(path).toBe("/Applications/Hisar.app/Contents/Helpers:/usr/bin:/bin");
    expect(prependToPath(path, location)).toBe(path);
    expect(prependToPath(undefined, location)).toBe("/Applications/Hisar.app/Contents/Helpers");
  });

  test("leaves PATH alone when hisar is already on it", () => {
    expect(prependToPath("/usr/bin", { path: "/usr/bin/hisar", isOnInheritedPath: true })).toBe("/usr/bin");
  });
});

describe("describePathNote", () => {
  test("adds nothing when the shell finds hisar on PATH", () => {
    expect(describePathNote({ path: "/usr/bin/hisar", isOnInheritedPath: true })).toBeUndefined();
  });

  test("names hisar's full path when the shell's PATH lacks it", () => {
    expect(describePathNote({ path: homeHelper, isOnInheritedPath: false })).toEndWith(
      `call hisar as \`${homeHelper}\`.`,
    );
  });
});
