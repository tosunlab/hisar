export const defaultServer = "https://eu.tryhisar.com";

export function canonicalServer(value: string): string {
  const text = value.trim() || defaultServer;
  try {
    const url = new URL(text.includes("://") ? text : `https://${text}`);
    if (!["https:", "http:"].includes(url.protocol) || url.username || url.password || url.search || url.hash) return "";
    return `${url.origin}${url.pathname.replace(/\/+$/, "")}`;
  } catch {
    return "";
  }
}

export type SignUpInvitation = { server: string; token: string; email: string };

export function parseSignUpInvitation(value: string, server: string): SignUpInvitation | null {
  const text = value.trim();
  if (/^[A-Za-z0-9_-]+$/.test(text)) return { server: canonicalServer(server), token: text, email: "" };
  try {
    const url = new URL(text);
    const match = url.pathname.match(/^(.*)\/invite\/([A-Za-z0-9_-]+)\/?$/);
    if (!match || !["https:", "http:"].includes(url.protocol) || url.username || url.password) return null;
    return { server: canonicalServer(`${url.origin}${match[1]}`), token: match[2], email: url.searchParams.get("email") ?? "" };
  } catch {
    return null;
  }
}
