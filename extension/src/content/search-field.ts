import { getParentAcrossShadow, isInputElement } from "./dom";

// A search box is never filled, whatever form it sits in: it takes a query, not a value from the vault. Its shape gives it
// away before its words do: the search input type, a search or combobox role, a datalist or typeahead, a search keyboard
// hint, or a place inside the page's search landmark.
const allSearchRole = new Set(["searchbox", "combobox"]);
const allSearchKeyHint = new Set(["search", "go"]);

// The words a search box is named with, in the languages the other detectors read, plus the names search forms give their
// one field. "Ara" is Turkish for search, so its bare word counts, as "q" and "s" do for a name alone.
const searchHintPattern =
  /\bsearch|\bquery|keyword|\bfilter|look\s*up|\bara(ma|y[ıi]n|y[ıi]n[ıi]z)?\b|\bsuche|\bsuchen\b|recherch|\bbuscar\b|b[uú]squeda|\bbusca\b|\bcerca\b|\bricerca\b|\bzoek|\bs[øo]k\b|\bsök\b|\bhaku\b|\bszukaj\b|поиск|検索|搜索|검색/i;
const searchNamePattern = /^(q|s|query|search|keyword|keywords|term|terms|search_query|searchterm|search-term)$/i;
const searchPathPattern = /(^|\/)(search|find|results|s)(\/|$)/i;

function getHints(input: HTMLInputElement): string {
  const allLabelText = Array.from(input.labels ?? [], (label) => (label.textContent ?? "").slice(0, 80));
  return [input.name, input.id, input.getAttribute("placeholder"), input.getAttribute("aria-label"), input.getAttribute("title"), ...allLabelText]
    .filter((hint): hint is string => Boolean(hint))
    .join(" ");
}

function isInsideSearchLandmark(input: HTMLInputElement): boolean {
  for (let current: Element | null = input; current; current = getParentAcrossShadow(current)) {
    if (current.getAttribute("role") === "search" || current.tagName === "SEARCH") return true;
  }
  return false;
}

// isSearchField reports whether an input is a search or filter box, which Hisar never fills and never shows the menu in.
export function isSearchField(element: Element): boolean {
  if (!isInputElement(element)) return false;
  const type = (element.getAttribute("type") ?? "").toLowerCase();
  if (type === "search") return true;
  if (type !== "" && type !== "text") return false;
  const role = (element.getAttribute("role") ?? "").toLowerCase();
  if (allSearchRole.has(role)) return true;
  if (element.hasAttribute("list") || element.getAttribute("aria-autocomplete") === "list" || element.getAttribute("aria-autocomplete") === "both") return true;
  if (allSearchKeyHint.has((element.getAttribute("enterkeyhint") ?? "").toLowerCase()) || element.getAttribute("inputmode") === "search") return true;
  if (isInsideSearchLandmark(element)) return true;
  if (element.form && searchPathPattern.test(element.form.getAttribute("action") ?? "")) return true;
  if (searchNamePattern.test(element.name) || searchNamePattern.test(element.id)) return true;
  return searchHintPattern.test(getHints(element));
}
