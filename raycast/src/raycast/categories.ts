import { Icon } from "@raycast/api";

export interface Category {
  label: string;
  icon: Icon;
}

/** The categories of core/item; an app newer than this extension may send others. */
const categoryByName: Record<string, Category> = {
  login: { label: "Login", icon: Icon.Key },
  "secure-note": { label: "Secure note", icon: Icon.Document },
  "ssh-key": { label: "SSH key", icon: Icon.Terminal },
  "credit-card": { label: "Credit card", icon: Icon.CreditCard },
  identity: { label: "Identity", icon: Icon.Person },
  address: { label: "Address", icon: Icon.House },
  "api-credential": { label: "API credential", icon: Icon.Code },
  "software-license": { label: "Software license", icon: Icon.Receipt },
  server: { label: "Server", icon: Icon.Network },
  database: { label: "Database", icon: Icon.HardDrive },
};

export function getCategory(name: string): Category {
  return categoryByName[name] ?? { label: name, icon: Icon.Lock };
}
