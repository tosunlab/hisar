import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";

const loginPage = (frameUrl: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Test login</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:280px} input{padding:8px;font-size:14px} iframe{margin-top:24px;width:340px;height:160px;border:1px dashed #999}</style>
</head><body>
<h1>Sign in to the test site</h1>
<form id="login" method="post" action="/welcome">
  <input id="username" name="username" autocomplete="username" placeholder="Email">
  <input id="password" name="password" type="password" autocomplete="current-password" placeholder="Password">
  <button id="sign-in" type="submit">Sign in</button>
</form>
<iframe id="other-origin" src="${frameUrl}"></iframe>
</body></html>`;

const addressForm = (prefix: string) => `<form id="${prefix}address" method="post" action="/welcome">
  <input id="${prefix}line1" autocomplete="shipping address-line1" placeholder="Address">
  <input id="${prefix}line2" autocomplete="shipping address-line2" placeholder="Apartment, suite">
  <input id="${prefix}city" autocomplete="shipping address-level2" placeholder="City">
  <input id="${prefix}region" autocomplete="shipping address-level1" placeholder="State">
  <input id="${prefix}postal" autocomplete="shipping postal-code" placeholder="ZIP code">
  <input id="${prefix}off-page-street" autocomplete="shipping street-address" style="position:absolute;left:-9999px;top:0">
  <input id="${prefix}transparent-street" autocomplete="shipping street-address" style="opacity:0;position:absolute">
  <select id="${prefix}country" autocomplete="shipping country"><option value="">Country</option><option>Germany</option><option>United States</option></select>
  <button type="submit">Ship here</button>
</form>`;

const addressPage = (sameOriginFrameUrl: string, crossOriginFrameUrl: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Checkout</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:280px} input,select{padding:8px;font-size:14px} iframe{margin-top:24px;width:340px;height:360px;border:1px dashed #999}</style>
</head><body>
<h1>Where should we send it?</h1>
${addressForm("")}
<iframe id="same-origin" src="${sameOriginFrameUrl}"></iframe>
<iframe id="other-origin" src="${crossOriginFrameUrl}"></iframe>
</body></html>`;

const addressFramePage = `<!doctype html>
<html><head><meta charset="utf-8"><style>form{display:grid;gap:8px;width:280px} input,select{padding:8px;font-size:14px}</style></head><body>
${addressForm("frame-")}
</body></html>`;

const identityPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Your details</title>
<style>body{font-family:sans-serif;margin:40px} form{display:grid;gap:8px;width:280px} input,select{padding:8px;font-size:14px}</style>
</head><body>
<h1>Your details</h1>
<form method="post" action="/welcome">
  <input id="full-name" autocomplete="name" placeholder="Full name">
  <input id="email" type="email" autocomplete="email" placeholder="Email">
  <input id="phone" type="tel" autocomplete="tel" placeholder="Phone">
  <input id="street" autocomplete="street-address" placeholder="Address">
  <input id="city" autocomplete="address-level2" placeholder="City">
  <input id="postal" autocomplete="postal-code" placeholder="ZIP code">
  <button type="submit">Continue</button>
</form>
</body></html>`;

const cardForm = (prefix: string) => `<form action="/paid" method="post">
<input id="${prefix}card-number" name="cardnumber" autocomplete="cc-number" placeholder="Card number">
<input id="${prefix}card-name" autocomplete="cc-name" placeholder="Name on card">
<select id="${prefix}card-month" name="exp_month"><option value="">MM</option><option value="11">11</option><option value="12">12</option></select>
<select id="${prefix}card-year" name="exp_year"><option value="">YYYY</option><option value="2029">2029</option><option value="2030">2030</option></select>
<input id="${prefix}card-code" name="cvc" type="password" placeholder="CVC">
<button>Pay</button>
</form>`;

const cardPage = (crossOriginFrameUrl: string) => `<!doctype html>
<html><head><meta charset="utf-8"><title>Pay</title>
<style>body{font-family:sans-serif;margin:40px} form{display:grid;gap:8px;width:280px} input,select{padding:8px;font-size:14px} iframe{margin-top:24px;width:340px;height:320px;border:1px dashed #999}</style>
</head><body>
<h1>Pay for your order</h1>
${cardForm("")}
<iframe id="payment-frame" src="${crossOriginFrameUrl}"></iframe>
</body></html>`;

const cardFramePage = `<!doctype html>
<html><head><meta charset="utf-8"><style>form{display:grid;gap:8px;width:280px} input,select{padding:8px;font-size:14px}</style></head><body>
${cardForm("frame-")}
</body></html>`;

const framePage = `<!doctype html>
<html><head><meta charset="utf-8"></head><body>
<form><input id="frame-username" autocomplete="username"><input id="frame-password" type="password"><button>Sign in</button></form>
</body></html>`;

// Another site (the cross origin) that frames the test site's login frame.
const outerFramePage = (innerUrl: string) => `<!doctype html>
<html><head><meta charset="utf-8"></head><body><iframe id="inner" src="${innerUrl}"></iframe></body></html>`;

const signUpPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Create an account</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:280px} input{padding:8px;font-size:14px}</style>
</head><body>
<h1>Create an account</h1>
<form id="sign-up" method="post" action="/welcome">
  <input id="email" name="email" type="email" autocomplete="email" placeholder="Email">
  <input id="new-password" name="password" type="password" autocomplete="new-password" placeholder="Password">
  <input id="confirm-password" name="confirm" type="password" autocomplete="new-password" placeholder="Repeat the password">
  <button type="submit">Create account</button>
</form>
</body></html>`;

const twoFactorPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Two-factor authentication</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form{display:grid;gap:8px;width:280px} input{padding:8px;font-size:14px}</style>
</head><body>
<h1>Two-factor authentication</h1>
<form id="two-factor" method="post" action="/welcome">
  <label for="app_totp">Authentication code</label>
  <input id="app_totp" name="app_otp" inputmode="numeric" autocomplete="one-time-code" placeholder="XXXXXX">
  <button type="submit">Verify</button>
</form>
</body></html>`;

const splitTwoFactorPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Enter the code</title>
<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} .boxes{display:flex;gap:8px} .boxes input{width:40px;height:48px;font-size:22px;text-align:center}</style>
</head><body>
<h1>Enter the 6-digit code</h1>
<form id="two-factor" method="post" action="/welcome">
  <div class="boxes">${Array.from({ length: 6 }, (_, index) => `<input id="digit-${index}" maxlength="1" inputmode="numeric" aria-label="Digit ${index + 1}">`).join("")}</div>
  <p><button type="submit">Verify</button></p>
</form>
<script>
for (const box of document.querySelectorAll(".boxes input")) {
  box.addEventListener("input", () => { if (box.value && box.nextElementSibling) box.nextElementSibling.focus(); });
}
</script>
</body></html>`;

const welcomePage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Welcome</title><style>body{font-family:sans-serif;background:#f4f4f5;margin:40px}</style></head>
<body><h1>Welcome back</h1><p>You are signed in.</p></body></html>`;

const pageStyle = `<style>body{font-family:sans-serif;background:#f4f4f5;margin:40px} form,.form{display:grid;gap:8px;width:280px} input{padding:8px;font-size:14px}</style>`;
const loginFields = `<input id="username" name="username" autocomplete="username" placeholder="Email">
  <input id="password" name="password" type="password" autocomplete="current-password" placeholder="Password">
  <button id="sign-in" type="submit">Sign in</button>`;

// The login form in a modal dialog: the dialog is in the top layer and the rest of the page is inert.
const dialogLoginPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Dialog login</title>${pageStyle}</head><body>
<h1>A page behind the dialog</h1>
<dialog id="login-dialog"><form id="login" method="post" action="/welcome">${loginFields}</form></dialog>
<script>document.getElementById("login-dialog").showModal();</script>
</body></html>`;

// Like a framework that renders the form again on each change: a change to the username replaces the password field.
const rerenderLoginPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Re-rendered login</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<form id="login" method="post" action="/welcome"></form>
<script>
const form = document.getElementById("login");
const model = { username: "", password: "" };
function render() {
  const active = document.activeElement?.id;
  form.innerHTML = ${"`"}${loginFields}${"`"};
  for (const name of ["username", "password"]) {
    const input = document.getElementById(name);
    input.value = model[name];
    input.addEventListener("input", () => { model[name] = input.value; if (name === "username") render(); });
  }
  if (active) document.getElementById(active)?.focus();
}
render();
</script>
</body></html>`;

// Like an input mask that keeps its own copy of the value from key events and writes it back on blur.
const maskedLoginPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Masked login</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<form id="login" method="post" action="/welcome">${loginFields}</form>
<script>
for (const input of document.querySelectorAll("input")) {
  let kept = "";
  input.addEventListener("keyup", () => { kept = input.value; });
  input.addEventListener("blur", () => { input.value = kept; });
}
</script>
</body></html>`;

// The "read-only until focused" trick some sites use against autofill.
const readOnlyLoginPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Read-only login</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<form id="login" method="post" action="/welcome">${loginFields}</form>
<script>
for (const input of document.querySelectorAll("input")) {
  input.readOnly = true;
  input.addEventListener("focus", () => setTimeout(() => { input.readOnly = false; }));
}
</script>
</body></html>`;

const lateLoginPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Late login</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<div id="slot"></div>
<script>setTimeout(() => { document.getElementById("slot").innerHTML = ${"`"}<form id="login" method="post" action="/welcome">${loginFields}</form>${"`"}; }, 1000);</script>
</body></html>`;

// Dark Reader's filter mode inverts the whole page with a filter on the root element.
const filteredLoginPage = `<!doctype html>
<html style="filter: invert(1) hue-rotate(180deg)"><head><meta charset="utf-8"><title>Filtered login</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<form id="login" method="post" action="/welcome">${loginFields}</form>
</body></html>`;

// The first step of a login without a form element, above a newsletter form in the footer.
const formlessWithNewsletterPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>Sign in</title>${pageStyle}</head><body>
<h1>Sign in</h1>
<div class="form"><input id="username" type="email" autocomplete="username" placeholder="Email"><div role="button" id="next">Next</div></div>
<footer><form action="/welcome" method="post"><input id="footer-email" type="email" name="email" placeholder="Email address"><button>Send me offers</button></form></footer>
</body></html>`;

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
    if (request.url?.startsWith("/dialog-login")) return response.end(dialogLoginPage);
    if (request.url?.startsWith("/rerender-login")) return response.end(rerenderLoginPage);
    if (request.url?.startsWith("/masked-login")) return response.end(maskedLoginPage);
    if (request.url?.startsWith("/readonly-login")) return response.end(readOnlyLoginPage);
    if (request.url?.startsWith("/late-login")) return response.end(lateLoginPage);
    if (request.url?.startsWith("/filtered-login")) return response.end(filteredLoginPage);
    if (request.url?.startsWith("/formless-with-newsletter")) return response.end(formlessWithNewsletterPage);
    if (request.url?.startsWith("/identity")) return response.end(identityPage);
    if (request.url?.startsWith("/card-frame")) return response.end(cardFramePage);
    if (request.url?.startsWith("/card")) return response.end(cardPage(`http://localhost:${port}/card-frame`));
    if (request.url?.startsWith("/address-frame")) return response.end(addressFramePage);
    if (request.url?.startsWith("/address")) return response.end(addressPage(`http://127.0.0.1:${port}/address-frame`, `http://localhost:${port}/address-frame`));
    if (request.url?.startsWith("/frame")) return response.end(framePage);
    if (request.url?.startsWith("/outer-frame")) return response.end(outerFramePage(`http://127.0.0.1:${port}/frame`));
    if (request.url?.startsWith("/sign-up")) return response.end(signUpPage);
    if (request.url?.startsWith("/two-factor-split")) return response.end(splitTwoFactorPage);
    if (request.url?.startsWith("/two-factor")) return response.end(twoFactorPage);
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
