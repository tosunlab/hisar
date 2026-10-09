import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";

const loginPage = (frameUrl: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Test login</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:280px} iframe{margin-top:24px;width:340px;height:160px;border:1px dashed #999}</style>
</head><body>
<h1>Sign in to the test site</h1>
<form id="login" method="post" action="/welcome">
  <input id="username" name="username" autocomplete="username" placeholder="Email">
  <input id="password" name="password" type="password" autocomplete="current-password" placeholder="Password">
  <button id="sign-in" type="submit">Sign in</button>
</form>
<iframe id="other-origin" src="${frameUrl}"></iframe>
</body></html>`;

const framePage = `<!doctype html>
<html><head><meta charset="utf-8"></head><body>
<form><input id="frame-username" autocomplete="username"><input id="frame-password" type="password"><button>Sign in</button></form>
</body></html>`;

const welcomePage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Welcome</title><style>body{font-family:sans-serif;background:#f4f4f5;margin:40px}</style></head>
<body><h1>Welcome back</h1><p>You are signed in.</p></body></html>`;

export interface TestSite {
  origin: string;
  crossOrigin: string;
  close: () => Promise<void>;
}

export async function startTestSite(): Promise<TestSite> {
  let port = 0;
  const server: Server = createServer((request, response) => {
    response.setHeader("Content-Type", "text/html; charset=utf-8");
    if (request.method === "POST") {
      request.resume();
      request.on("end", () => response.end(welcomePage));
      return;
    }
    if (request.url?.startsWith("/frame")) return response.end(framePage);
    response.end(loginPage(`http://localhost:${port}/frame`));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  port = (server.address() as AddressInfo).port;
  return {
    origin: `http://127.0.0.1:${port}`,
    crossOrigin: `http://localhost:${port}`,
    close: () => new Promise<void>((resolve) => server.close(() => resolve())),
  };
}
