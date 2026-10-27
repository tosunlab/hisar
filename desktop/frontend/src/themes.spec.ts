import { describe, expect, it } from "vitest";
import themeSource from "../../../assets/themes/themes.json";
import generatedCSS from "./themes.css?raw";
import generatedAdminCSS from "../../../admin/src/themes.css?raw";
import generatedSwift from "../../../mobile/ios/Shared/Views/ThemePalettes.swift?raw";
import generatedKotlin from "../../../mobile/android/model/src/main/kotlin/com/tryhisar/hisar/model/theme/ThemePalettes.kt?raw";
import detailSource from "./components/ItemDetailPanel.vue?raw";
import codeSource from "./components/OneTimeCodeField.vue?raw";
import { computeContrastRatio, renderThemeCSS, renderThemeKotlin, renderThemeSwift, resolveAllTheme, type ThemeSource } from "../../../scripts/themes/render";

const source = themeSource as ThemeSource;
const allTheme = resolveAllTheme(source);
const textMinimum = 4.5;
const interfaceMinimum = 3;

interface ContrastRule {
  allForeground: string[];
  allBackground: string[];
  minimum: number;
}

const allTileIndex = Array.from({ length: 10 }, (_, index) => index);

const allRule: ContrastRule[] = [
  { allForeground: ["text", "textSecondary"], allBackground: ["background", "sidebar", "panel", "surface", "surfaceRaised", "hover", "selected", "code"], minimum: textMinimum },
  { allForeground: ["textMuted"], allBackground: ["background", "sidebar", "panel", "surface", "surfaceRaised", "code"], minimum: textMinimum },
  { allForeground: ["accentText"], allBackground: ["background", "sidebar", "panel", "surface"], minimum: textMinimum },
  { allForeground: ["accentText"], allBackground: ["selected"], minimum: interfaceMinimum },
  { allForeground: ["text"], allBackground: ["textSelection"], minimum: textMinimum },
  { allForeground: ["primaryText"], allBackground: ["primary", "primaryHover"], minimum: textMinimum },
  { allForeground: ["onDanger"], allBackground: ["dangerSolid"], minimum: textMinimum },
  { allForeground: ["danger", "warning", "success", "successText"], allBackground: ["background", "panel", "surface"], minimum: textMinimum },
  { allForeground: ["dangerText"], allBackground: ["dangerBackground"], minimum: textMinimum },
  { allForeground: ["warning", "warningText"], allBackground: ["warningBackground"], minimum: textMinimum },
  { allForeground: ["danger"], allBackground: ["dangerBackground"], minimum: interfaceMinimum },
  { allForeground: ["focus", "primary"], allBackground: ["background", "sidebar", "panel", "surface"], minimum: interfaceMinimum },
  ...allTileIndex.map((index) => ({ allForeground: [`tile${index}Text`], allBackground: [`tile${index}`], minimum: textMinimum })),
  { allForeground: ["iosTint", "iosSecondaryLabel", "iosWarning"], allBackground: ["iosGroupedBackground", "iosCellBackground"], minimum: textMinimum },
  { allForeground: ["dangerText"], allBackground: ["iosGroupedBackground", "iosCellBackground"], minimum: textMinimum },
  { allForeground: ["iosOnTint"], allBackground: ["iosTint"], minimum: textMinimum },
];

describe("themes", () => {
  it("defines every token for every colour and appearance", () => {
    const allLightName = Object.keys(source.base.light).sort();
    expect(Object.keys(source.base.dark).sort()).toEqual(allLightName);
    for (const theme of allTheme) {
      expect(Object.keys(theme.tokens).sort(), `${theme.color} ${theme.appearance}`).toEqual(allLightName);
    }
    expect(allTheme).toHaveLength(8);
  });

  it.each(allTheme.map((theme) => [theme.color, theme.appearance, theme.tokens] as const))("%s %s meets the WCAG contrast minimums", (_color, _appearance, tokens) => {
    const allFailure: string[] = [];
    for (const rule of allRule) {
      for (const foreground of rule.allForeground) {
        for (const background of rule.allBackground) {
          const ratio = computeContrastRatio(tokens[foreground], tokens[background]);
          if (ratio < rule.minimum) {
            allFailure.push(`${foreground} ${tokens[foreground]} on ${background} ${tokens[background]}: ${ratio.toFixed(2)} < ${rule.minimum}`);
          }
        }
      }
    }
    expect(allFailure).toEqual([]);
  });

  it.each([["item fields", detailSource], ["one-time codes", codeSource]])("keeps enabled %s actions legible at rest and during interaction", (_name, componentSource) => {
    const style = document.createElement("style");
    style.textContent = componentSource.match(/<style scoped>([\s\S]*?)<\/style>/)![1]!;
    const row = document.createElement("div");
    row.className = "field";
    row.innerHTML = '<span class="field-actions"><button class="ghost">Copy</button></span>';
    document.head.append(style);
    document.body.append(row);
    const actions = row.querySelector<HTMLElement>(".field-actions")!;
    const opacity = Number(getComputedStyle(actions).opacity || "1");
    expect(opacity).toBe(1);
    for (const theme of allTheme.filter((theme) => theme.appearance === "light")) {
      expect(computeContrastRatio(theme.tokens.textSecondary!, theme.tokens.panel!), theme.color).toBeGreaterThanOrEqual(interfaceMinimum);
    }
    row.querySelector("button")!.focus();
    expect(Number(getComputedStyle(actions).opacity || "1")).toBe(1);
    row.remove();
    style.remove();
  });

  it("computes the WCAG ratio", () => {
    expect(computeContrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(computeContrastRatio("#ffffff", "#ffffff")).toBe(1);
    expect(computeContrastRatio("#767676", "#ffffff")).toBeCloseTo(4.54, 2);
  });

  it("keeps the generated CSS, Swift and Kotlin up to date with themes.json (run make generate-themes)", () => {
    expect(generatedCSS).toBe(renderThemeCSS(source));
    expect(generatedAdminCSS).toBe(renderThemeCSS(source));
    expect(generatedSwift).toBe(renderThemeSwift(source));
    expect(generatedKotlin).toBe(renderThemeKotlin(source));
  });
});
