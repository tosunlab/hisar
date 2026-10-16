import { beforeEach, describe, expect, it } from "vitest";
import autocompletePage from "./fixtures/address-autocomplete.html?raw";
import hiddenTrapPage from "./fixtures/address-hidden-trap.html?raw";
import selectsPage from "./fixtures/address-selects.html?raw";
import unhintedPage from "./fixtures/address-unhinted.html?raw";
import simpleLoginPage from "./fixtures/simple-login.html?raw";
import { classifyAddressField, findAddressScopeOf, findAllAddressScope } from "./address-detection";

function getElement(selector: string): Element {
  const element = document.querySelector(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

function describeScope(selector: string): { id: string; kind: string; group: string }[] {
  const scope = findAddressScopeOf(document, getElement(selector));
  return (scope?.allField ?? []).map((field) => ({ id: field.element.id, kind: field.kind, group: field.group }));
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("finding address fields", () => {
  it("reads autocomplete tokens and keeps shipping and billing apart", () => {
    document.body.innerHTML = autocompletePage;
    expect(describeScope("#ship-city")).toEqual([
      { id: "ship-line1", kind: "line1", group: "shipping" },
      { id: "ship-line2", kind: "line2", group: "shipping" },
      { id: "ship-city", kind: "city", group: "shipping" },
      { id: "ship-region", kind: "region", group: "shipping" },
      { id: "ship-postal", kind: "postal", group: "shipping" },
      { id: "ship-country", kind: "country", group: "shipping" },
      { id: "bill-street", kind: "street", group: "billing" },
      { id: "bill-city", kind: "city", group: "billing" },
      { id: "bill-postal", kind: "postal", group: "billing" },
    ]);
    expect(classifyAddressField(getElement("#ship-name"))).toBeUndefined();
    expect(classifyAddressField(getElement("#ship-email"))).toBeUndefined();
  });

  it("reads labels when a form has no autocomplete attributes, and leaves an email address alone", () => {
    document.body.innerHTML = unhintedPage;
    expect(describeScope("#field-1").map((field) => field.kind)).toEqual(["street", "city", "postal", "country"]);
    expect(findAddressScopeOf(document, getElement("#newsletter-email"))).toBeUndefined();
  });

  it("groups fields outside a form that each sit in their own label or row, and keeps two such sections apart", () => {
    document.body.innerHTML = `
      <main>
        <section id="delivery">
          <label>Street <input id="d-street" autocomplete="street-address" /></label>
          <div class="row">
            <label>City <input id="d-city" autocomplete="address-level2" /></label>
            <label>Postal code <input id="d-postal" autocomplete="postal-code" /></label>
          </div>
          <div><label>Country <input id="d-country" autocomplete="country" /></label></div>
        </section>
        <section id="pickup">
          <div><label>Address <input id="p-street" /></label></div>
          <div><label>Town <input id="p-city" /></label></div>
        </section>
      </main>`;
    expect(describeScope("#d-city").map((field) => field.id)).toEqual(["d-street", "d-city", "d-postal", "d-country"]);
    expect(findAddressScopeOf(document, getElement("#d-street"))?.root).toBe(getElement("#delivery"));
    expect(describeScope("#p-city").map((field) => field.id)).toEqual(["p-street", "p-city"]);
    expect(findAllAddressScope(document)).toHaveLength(2);
  });

  it("takes country and region lists, and a list of saved addresses is no street", () => {
    document.body.innerHTML = selectsPage;
    expect(describeScope("#street").map((field) => field.kind)).toEqual(["street", "region", "country", "country"]);
    document.body.innerHTML = `<form><select id="saved" name="saved_address"><option>Home</option></select><input id="city" autocomplete="address-level2" /></form>`;
    expect(classifyAddressField(getElement("#saved"))).toBeUndefined();
  });

  it("never counts a hidden, disabled or read-only field", () => {
    document.body.innerHTML = hiddenTrapPage;
    expect(describeScope("#visible-postal").map((field) => field.id)).toEqual(["visible-postal"]);
    for (const id of ["hidden-street", "hidden-city", "disabled-city", "readonly-country"]) {
      expect(classifyAddressField(getElement(`#${id}`))).toBeUndefined();
    }
  });

  it("needs a field named by autocomplete or two kinds found by their labels", () => {
    document.body.innerHTML = `<form><label for="a">Address</label><input id="a" /><button>Go</button></form>`;
    expect(findAllAddressScope(document)).toEqual([]);
    document.body.innerHTML = simpleLoginPage;
    expect(findAllAddressScope(document)).toEqual([]);
  });
});
