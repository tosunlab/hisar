import { beforeEach, describe, expect, it } from "vitest";
import autocompletePage from "./fixtures/address-autocomplete.html?raw";
import hiddenTrapPage from "./fixtures/address-hidden-trap.html?raw";
import selectsPage from "./fixtures/address-selects.html?raw";
import unhintedPage from "./fixtures/address-unhinted.html?raw";
import { fillAddressIntoPage, fillAddressIntoScopeOf, hasAddressForm, isFocusedAddressField } from "./address-fill";
import type { AddressValues } from "../shared/protocol";

const home: AddressValues = { street: "1 Main St\nApt 4\nBuilding B", city: "Springfield", region: "IL", postalCode: "62704", country: "US" };

function getValue(selector: string): string {
  const element = document.querySelector<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element.value;
}

function getElement(selector: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("filling an address", () => {
  it("fills the anchored field's group, splitting the street into its lines", () => {
    document.body.innerHTML = autocompletePage;
    const allEvent: string[] = [];
    document.addEventListener("change", (event) => allEvent.push((event.target as HTMLElement).id));
    expect(fillAddressIntoScopeOf(document, getElement("#ship-line1"), home)).toBe(6);
    expect(getValue("#ship-line1")).toBe("1 Main St");
    expect(getValue("#ship-line2")).toBe("Apt 4, Building B");
    expect(getValue("#ship-city")).toBe("Springfield");
    expect(getValue("#ship-region")).toBe("IL");
    expect(getValue("#ship-postal")).toBe("62704");
    expect(getValue("#ship-country")).toBe("US");
    expect(getValue("#bill-street")).toBe("");
    expect(getValue("#ship-name")).toBe("");
    expect(allEvent).toContain("ship-country");
    expect(document.activeElement).toBe(getElement("#ship-line1"));
  });

  it("keeps the street's lines in a street-address textarea", () => {
    document.body.innerHTML = autocompletePage;
    fillAddressIntoScopeOf(document, getElement("#bill-city"), home);
    expect(getValue("#bill-street")).toBe("1 Main St\nApt 4\nBuilding B");
    expect(getValue("#ship-line1")).toBe("");
  });

  it("joins the street into one line for a street-address input, and fills lists by code or name", () => {
    document.body.innerHTML = selectsPage;
    fillAddressIntoScopeOf(document, getElement("#street"), home);
    expect(getValue("#street")).toBe("1 Main St, Apt 4, Building B");
    expect(getValue("#region")).toBe("IL");
    expect(getValue("#country-name")).toBe("United States");
    expect(getValue("#country-text")).toBe("US");

    document.body.innerHTML = selectsPage;
    fillAddressIntoScopeOf(document, getElement("#street"), { ...home, region: "Ontario", country: "United States" });
    expect(getValue("#region")).toBe("ON");
    expect(getValue("#country-text")).toBe("US");
  });

  it("matches a country list written in another language or with accents", () => {
    document.body.innerHTML = autocompletePage;
    fillAddressIntoScopeOf(document, getElement("#ship-city"), { ...home, country: "Turkiye" });
    expect(getValue("#ship-country")).toBe("TR");
  });

  it("never replaces what the user typed, and fills only fields it can see", () => {
    document.body.innerHTML = hiddenTrapPage;
    expect(fillAddressIntoScopeOf(document, getElement("#visible-postal"), home)).toBe(1);
    expect(getValue("#hidden-street")).toBe("");
    expect(getValue("#hidden-city")).toBe("");
    expect(getValue("#transparent-street")).toBe("");
    expect(getValue("#faded-region")).toBe("");
    expect(getValue("#disabled-city")).toBe("");

    document.body.innerHTML = unhintedPage;
    (getElement("#field-2") as HTMLInputElement).value = "Chicago";
    expect(fillAddressIntoScopeOf(document, getElement("#field-1"), home)).toBe(3);
    expect(getValue("#field-1")).toBe("1 Main St, Apt 4, Building B");
    expect(getValue("#field-2")).toBe("Chicago");
    expect(getValue("#field-4")).toBe("US");
  });

  it("fills the focused form from the popup, or else the first one, and reports what it found", () => {
    document.body.innerHTML = autocompletePage;
    expect(hasAddressForm(document)).toBe(true);
    expect(isFocusedAddressField(document)).toBe(false);
    getElement("#bill-postal").focus();
    expect(isFocusedAddressField(document)).toBe(true);
    fillAddressIntoPage(document, home);
    expect(getValue("#bill-postal")).toBe("62704");
    expect(getValue("#ship-postal")).toBe("");

    document.body.innerHTML = `<form><input id="other" /></form>`;
    expect(hasAddressForm(document)).toBe(false);
    expect(fillAddressIntoPage(document, home)).toBe(0);
  });
});
