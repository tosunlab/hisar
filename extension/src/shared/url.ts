export function isWebUrl(url: string | undefined): url is string {
  if (!url) return false;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" || parsed.protocol === "http:";
  } catch {
    return false;
  }
}

export function getOrigin(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return "null";
  }
}

export function getHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}

export function getBrowserName(userAgentData: { brands?: { brand: string }[] } | undefined, userAgent: string, isBrave: boolean): string {
  if (isBrave) return "Brave";
  const allBrand = (userAgentData?.brands ?? []).map((entry) => entry.brand);
  const allKnown: [string, string][] = [
    ["Microsoft Edge", "Edge"],
    ["Opera", "Opera"],
    ["Vivaldi", "Vivaldi"],
    ["Brave", "Brave"],
    ["Google Chrome", "Chrome"],
  ];
  for (const [brand, name] of allKnown) {
    if (allBrand.includes(brand)) return name;
  }
  if (/Edg\//.test(userAgent)) return "Edge";
  if (/Vivaldi\//.test(userAgent)) return "Vivaldi";
  if (/OPR\//.test(userAgent)) return "Opera";
  if (allBrand.includes("Chromium")) return "Chromium";
  return "Chrome";
}
