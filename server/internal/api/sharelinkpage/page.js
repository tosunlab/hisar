"use strict";
(() => {
  const minimumIterations = 600000;
  const maximumIterations = 10000000;
  const linkKeySize = 32;
  const saltSize = 16;
  const nonceSize = 12;
  const textEncoder = new TextEncoder();

  const allStep = Array.from(document.querySelectorAll("[data-step]"));
  const revealButton = document.getElementById("reveal-button");
  const passphraseForm = document.getElementById("passphrase-form");
  const passphraseInput = document.getElementById("passphrase-input");
  const passphraseButton = document.getElementById("passphrase-button");
  const passphraseError = document.getElementById("passphrase-error");
  const retryButton = document.getElementById("retry-button");
  const announcement = document.getElementById("announcement");

  let fetchedLink = null;

  function showStep(name) {
    for (const step of allStep) {
      step.hidden = step.dataset.step !== name;
    }
    const heading = document.querySelector(`[data-step="${name}"] h1`);
    if (heading && name !== "reveal") {
      heading.focus();
    }
  }

  function announce(message) {
    announcement.textContent = "";
    window.setTimeout(() => {
      announcement.textContent = message;
    }, 50);
  }

  function showError(title, detail, isRetryable) {
    document.getElementById("error-title").textContent = title;
    document.getElementById("error-detail").textContent = detail;
    retryButton.hidden = !isRetryable;
    showStep("error");
    announce(title);
  }

  function decodeBase64(text, isURLSafe) {
    let normalized = text;
    if (isURLSafe) {
      if (!/^[A-Za-z0-9_-]*$/.test(text)) {
        return null;
      }
      normalized = text.replace(/-/g, "+").replace(/_/g, "/");
      normalized += "=".repeat((4 - (normalized.length % 4)) % 4);
    }
    try {
      return Uint8Array.from(atob(normalized), (character) => character.charCodeAt(0));
    } catch {
      return null;
    }
  }

  function readLink() {
    const match = /^\/s\/([A-Za-z0-9_-]{22})\/?$/.exec(window.location.pathname);
    const linkKey = decodeBase64(window.location.hash.slice(1), true);
    if (!match || !linkKey || linkKey.length !== linkKeySize) {
      return null;
    }
    return { id: match[1], linkKey };
  }

  function concatenateBytes(first, second) {
    const combined = new Uint8Array(first.length + second.length);
    combined.set(first, 0);
    combined.set(second, first.length);
    return combined;
  }

  async function deriveContentKey(linkKey, passphrase, passphraseParams) {
    let inputKeyMaterial = linkKey;
    let info = "hisar/v1/share-link/content";
    if (passphraseParams) {
      const passwordKey = await crypto.subtle.importKey("raw", textEncoder.encode(passphrase.normalize("NFKD")), "PBKDF2", false, ["deriveBits"]);
      const passphraseBits = await crypto.subtle.deriveBits(
        { name: "PBKDF2", hash: "SHA-256", salt: passphraseParams.salt, iterations: passphraseParams.iterations },
        passwordKey,
        256,
      );
      inputKeyMaterial = concatenateBytes(linkKey, new Uint8Array(passphraseBits));
      info = "hisar/v1/share-link/content-with-passphrase";
    }
    const hkdfKey = await crypto.subtle.importKey("raw", inputKeyMaterial, "HKDF", false, ["deriveKey"]);
    return crypto.subtle.deriveKey(
      { name: "HKDF", hash: "SHA-256", salt: new Uint8Array(0), info: textEncoder.encode(info) },
      hkdfKey,
      { name: "AES-GCM", length: 256 },
      false,
      ["decrypt"],
    );
  }

  async function decryptPayload(contentKey, ciphertext) {
    if (ciphertext.length < 1 + nonceSize + 16 || ciphertext[0] !== 1) {
      throw new Error("unsupported ciphertext");
    }
    const plaintext = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: ciphertext.slice(1, 1 + nonceSize), additionalData: textEncoder.encode("hisar/v1/share-link") },
      contentKey,
      ciphertext.slice(1 + nonceSize),
    );
    const payload = JSON.parse(new TextDecoder().decode(plaintext));
    if (payload.version !== 1 || !Array.isArray(payload.fields)) {
      throw new Error("unsupported payload");
    }
    return payload;
  }

  function readPassphraseParams(passphrase) {
    if (!passphrase) {
      return null;
    }
    const salt = decodeBase64(passphrase.salt || "", false);
    const iterations = passphrase.iterations;
    if (!salt || salt.length !== saltSize || !Number.isInteger(iterations) || iterations < minimumIterations || iterations > maximumIterations) {
      throw new Error("unsafe passphrase parameters");
    }
    return { salt, iterations };
  }

  async function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return;
    }
    const scratch = document.createElement("textarea");
    scratch.value = text;
    scratch.setAttribute("readonly", "");
    scratch.className = "visually-hidden";
    document.body.append(scratch);
    scratch.select();
    const isCopied = document.execCommand("copy");
    scratch.remove();
    if (!isCopied) {
      throw new Error("copy failed");
    }
  }

  function createButton(text, label) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = text;
    button.setAttribute("aria-label", label);
    return button;
  }

  function renderField(field) {
    const container = document.createElement("div");
    container.className = "field";
    const term = document.createElement("dt");
    term.textContent = field.label || "Value";
    const description = document.createElement("dd");
    const isMultiline = field.kind === "notes" || field.kind === "text" || String(field.value).includes("\n");
    const valueElement = document.createElement(isMultiline ? "pre" : "span");
    valueElement.className = "field-value";
    if (isMultiline && String(field.value).length > 600) {
      valueElement.classList.add("is-long");
    }
    const value = String(field.value);
    const hiddenValue = "•".repeat(Math.min(Math.max(value.length, 8), 24));
    valueElement.textContent = field.isConcealed ? hiddenValue : value;
    const actions = document.createElement("div");
    actions.className = "field-actions";
    if (field.isConcealed) {
      const showButton = createButton("Show", `Show ${term.textContent}`);
      showButton.setAttribute("aria-pressed", "false");
      showButton.addEventListener("click", () => {
        const isShown = showButton.getAttribute("aria-pressed") === "true";
        valueElement.textContent = isShown ? hiddenValue : value;
        showButton.textContent = isShown ? "Show" : "Hide";
        showButton.setAttribute("aria-pressed", String(!isShown));
        showButton.setAttribute("aria-label", `${isShown ? "Show" : "Hide"} ${term.textContent}`);
      });
      actions.append(showButton);
    }
    const copyButton = createButton("Copy", `Copy ${term.textContent}`);
    copyButton.addEventListener("click", async () => {
      try {
        await copyText(value);
        copyButton.textContent = "Copied";
        announce(`${term.textContent} copied`);
        window.setTimeout(() => {
          copyButton.textContent = "Copy";
        }, 2000);
      } catch {
        announce("Couldn't copy; select the text and copy it instead");
      }
    });
    actions.append(copyButton);
    description.append(valueElement, actions);
    container.append(term, description);
    return container;
  }

  function describeLinkStatus(link) {
    const expiresAt = new Date(link.expiresAt);
    const expiry = Number.isNaN(expiresAt.getTime())
      ? ""
      : new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(expiresAt);
    if (link.maxViews > 0) {
      const viewsLeft = Math.max(link.maxViews - link.viewCount, 0);
      if (viewsLeft === 0) {
        return "That was the last view: this link no longer works. Copy what you need now; it disappears when you close this page.";
      }
      return `${viewsLeft} more ${viewsLeft === 1 ? "view" : "views"} allowed. The link expires on ${expiry}.`;
    }
    return `Anyone with the link can open it until ${expiry}.`;
  }

  function renderPayload(payload) {
    document.getElementById("secret-title").textContent = payload.title || "Shared with you";
    const fieldList = document.getElementById("field-list");
    fieldList.replaceChildren(...payload.fields.map(renderField));
    document.getElementById("link-status").textContent = describeLinkStatus(fetchedLink);
    document.title = `${payload.title || "Shared with you"} · Hisar`;
    showStep("secret");
    announce("Secret revealed");
  }

  async function openFetchedLink(passphrase) {
    const contentKey = await deriveContentKey(fetchedLink.linkKey, passphrase, fetchedLink.passphraseParams);
    return decryptPayload(contentKey, fetchedLink.ciphertext);
  }

  async function reveal() {
    const link = readLink();
    if (!link) {
      showError("This link is incomplete", "The part after # is missing or damaged. Ask the sender to copy the whole link again.", false);
      return;
    }
    revealButton.disabled = true;
    revealButton.textContent = "Revealing…";
    let response;
    try {
      response = await fetch(`/api/v1/public/share-links/${link.id}`, { cache: "no-store", credentials: "omit", referrerPolicy: "no-referrer" });
    } catch {
      showError("Couldn't reach Hisar", "Check your connection and try again. No view was used.", true);
      return;
    }
    if (response.status === 404) {
      showError("This link has expired or was already viewed", "Share links work for a limited time and number of views. Ask the sender for a new link.", false);
      return;
    }
    if (response.status === 429) {
      showError("Too many attempts", "Wait a minute and try again.", true);
      return;
    }
    if (!response.ok) {
      showError("Something went wrong", "The server couldn't open this link right now. Try again in a moment.", true);
      return;
    }
    try {
      const body = await response.json();
      fetchedLink = {
        linkKey: link.linkKey,
        ciphertext: decodeBase64(body.ciphertext || "", false) || new Uint8Array(0),
        passphraseParams: readPassphraseParams(body.passphrase),
        expiresAt: body.expiresAt,
        maxViews: body.maxViews,
        viewCount: body.viewCount,
      };
    } catch {
      showError("This link can't be opened safely", "The server sent something this page doesn't accept. Ask the sender for a new link.", false);
      return;
    }
    window.history.replaceState(null, "", window.location.pathname);
    if (fetchedLink.passphraseParams) {
      showStep("passphrase");
      passphraseInput.focus();
      return;
    }
    try {
      renderPayload(await openFetchedLink(null));
    } catch {
      showError("This link can't be opened", "The key in the link doesn't match. Ask the sender for a new link.", false);
    }
  }

  async function submitPassphrase(event) {
    event.preventDefault();
    if (!passphraseInput.value) {
      passphraseError.textContent = "Enter the passphrase.";
      passphraseError.hidden = false;
      passphraseInput.setAttribute("aria-invalid", "true");
      return;
    }
    passphraseButton.disabled = true;
    passphraseButton.textContent = "Unlocking…";
    passphraseError.hidden = true;
    try {
      const payload = await openFetchedLink(passphraseInput.value);
      passphraseInput.value = "";
      renderPayload(payload);
    } catch {
      passphraseError.textContent = "That passphrase doesn't open this link. Check it and try again.";
      passphraseError.hidden = false;
      passphraseInput.setAttribute("aria-invalid", "true");
      passphraseInput.select();
    } finally {
      passphraseButton.disabled = false;
      passphraseButton.textContent = "Unlock";
    }
  }

  revealButton.addEventListener("click", reveal);
  passphraseForm.addEventListener("submit", submitPassphrase);
  retryButton.addEventListener("click", () => {
    revealButton.disabled = false;
    revealButton.textContent = "Reveal";
    showStep("reveal");
    revealButton.focus();
  });

  if (!window.crypto || !window.crypto.subtle) {
    showError("This browser can't open the link", "Open it in an up-to-date browser over https.", false);
  } else if (!readLink()) {
    showError("This link is incomplete", "The part after # is missing or damaged. Ask the sender to copy the whole link again.", false);
  }
})();
