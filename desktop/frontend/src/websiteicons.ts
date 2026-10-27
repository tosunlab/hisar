import { reactive, ref, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import { api, getErrorMessage } from "./api";

// At most this many website icon requests run at once; the backend may be asking the websites.
export const maxWebsiteIconRequests = 4;

// Icons by domain for this session, as data URLs ("" while loading or when the website has none).
const iconsByDomain = new Map<string, Ref<string>>();
const waitingDomains: { domain: string; url: string }[] = [];
let runningRequests = 0;

function getIconDomain(url: string): string {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" ? parsed.hostname.toLowerCase().replace(/\.$/, "") : "";
  } catch {
    return "";
  }
}

function startWaitingRequests(): void {
  while (runningRequests < maxWebsiteIconRequests && waitingDomains.length > 0) {
    const { domain, url } = waitingDomains.shift()!;
    runningRequests += 1;
    api
      .getWebsiteIcon(url)
      .then((icon) => {
        const iconRef = iconsByDomain.get(domain);
        if (iconRef && icon) {
          iconRef.value = `data:image/png;base64,${icon}`;
        }
      })
      .catch(() => undefined)
      .finally(() => {
        runningRequests -= 1;
        startWaitingRequests();
      });
  }
}

// getWebsiteIcon returns the data URL of a website's icon, which fills in once the backend has it; every tile of one
// domain shares a single request.
export function getWebsiteIcon(url: string): Ref<string> {
  const domain = getIconDomain(url);
  if (domain === "") {
    return ref("");
  }
  let iconRef = iconsByDomain.get(domain);
  if (!iconRef) {
    iconRef = ref("");
    iconsByDomain.set(domain, iconRef);
    waitingDomains.push({ domain, url });
    startWaitingRequests();
  }
  return iconRef;
}

// websiteIconState: generation grows whenever the icons are forgotten, so tiles ask again; isEnabled mirrors the setting.
export const websiteIconState = reactive({ generation: 0, isEnabled: true, errorMessage: "" });

// forgetWebsiteIcons drops this session's icons, after website icons were turned off or the app locked.
export function forgetWebsiteIcons(): void {
  for (const iconRef of iconsByDomain.values()) {
    iconRef.value = "";
  }
  iconsByDomain.clear();
  waitingDomains.length = 0;
  websiteIconState.generation += 1;
}

export async function loadWebsiteIconSetting(): Promise<void> {
  try {
    websiteIconState.isEnabled = await api.isWebsiteIconsEnabled();
  } catch {
    return;
  }
}

// startWebsiteIconSync loads the setting and follows changes made in any window, such as the macOS Settings window; it
// returns the stop function.
export function startWebsiteIconSync(): () => void {
  const removeListener = Events.On("website-icons-changed", (event) => {
    websiteIconState.isEnabled = event.data;
    forgetWebsiteIcons();
  });
  loadWebsiteIconSetting();
  return removeListener;
}

// setWebsiteIconsEnabled saves the setting; turning it off also forgets the icons the backend cached.
export async function setWebsiteIconsEnabled(isEnabled: boolean): Promise<void> {
  websiteIconState.errorMessage = "";
  try {
    await api.setWebsiteIconsEnabled(isEnabled);
    websiteIconState.isEnabled = isEnabled;
    forgetWebsiteIcons();
  } catch (error) {
    websiteIconState.errorMessage = getErrorMessage(error);
  }
}
