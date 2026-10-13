"use strict";
(() => {
  const announcement = document.getElementById("announcement");

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

  function announce(message) {
    if (announcement) {
      announcement.textContent = message;
    }
  }

  for (const button of document.querySelectorAll("button[data-copy]")) {
    const source = document.getElementById(button.dataset.copy);
    if (!source) {
      continue;
    }
    button.hidden = false;
    button.addEventListener("click", async () => {
      try {
        await copyText(source.textContent.trim());
        button.textContent = "Copied";
        announce("Copied");
        setTimeout(() => {
          button.textContent = "Copy";
        }, 2000);
      } catch {
        announce("Couldn't copy; select the text and copy it instead");
      }
    });
  }
})();
