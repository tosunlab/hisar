// Country codes and names for filling a country field, built from the browser's Intl data instead of a list of our own: a
// saved "US" fills a list that shows "United States", and a saved "Türkiye" fills one whose values are codes.

const allLetter = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
const namesByLocale = new Map<string, Map<string, string>>();

function normalize(text: string): string {
  return text
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .trim()
    .toLowerCase();
}

// listCountryNames maps every region code the browser names in locale to its name, such as "TR" to "Türkiye".
function listCountryNames(locale: string): Map<string, string> {
  const cached = namesByLocale.get(locale);
  if (cached) return cached;
  const names = new Map<string, string>();
  let displayNames: Intl.DisplayNames;
  try {
    displayNames = new Intl.DisplayNames([locale], { type: "region", fallback: "none" });
  } catch {
    namesByLocale.set(locale, names);
    return names;
  }
  for (const first of allLetter) {
    for (const second of allLetter) {
      const code = first + second;
      const name = displayNames.of(code);
      if (name && name !== code) names.set(code, name);
    }
  }
  namesByLocale.set(locale, names);
  return names;
}

function listLocale(): string[] {
  const allLocale = ["en"];
  const language = typeof navigator !== "undefined" ? navigator.language : "";
  if (language && !allLocale.includes(language)) allLocale.push(language);
  return allLocale;
}

// findCountryCode returns the two-letter code of a country written as a code or a name in English or the browser's
// language, or undefined.
export function findCountryCode(country: string): string | undefined {
  const normalized = normalize(country);
  if (!normalized) return undefined;
  for (const locale of listLocale()) {
    const names = listCountryNames(locale);
    if (normalized.length === 2 && names.has(normalized.toUpperCase())) return normalized.toUpperCase();
    for (const [code, name] of names) {
      if (normalize(name) === normalized) return code;
    }
  }
  return undefined;
}

// listCountrySpellings returns the ways a list may write a country: its code and its names in English and the browser's
// language, normalized for comparing with normalizeCountryText.
export function listCountrySpellings(country: string): string[] {
  const allSpelling = new Set([normalize(country)]);
  const code = findCountryCode(country);
  if (code) {
    allSpelling.add(normalize(code));
    for (const locale of listLocale()) {
      const name = listCountryNames(locale).get(code);
      if (name) allSpelling.add(normalize(name));
    }
  }
  allSpelling.delete("");
  return [...allSpelling];
}

// getCountryName names a country in the browser's language, falling back to English and then to the text as saved.
export function getCountryName(country: string): string {
  const code = findCountryCode(country);
  if (!code) return country;
  const allLocale = listLocale();
  return listCountryNames(allLocale[allLocale.length - 1]).get(code) ?? listCountryNames("en").get(code) ?? country;
}

export function normalizeCountryText(text: string): string {
  return normalize(text);
}
