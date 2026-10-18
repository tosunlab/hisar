import { beforeEach, describe, expect, it } from "vitest";
import type { IdentityValues } from "../shared/protocol";
import { findInlineMenuPurpose } from "./inline-menu/eligibility";
import { findAllIdentityScope } from "./identity-detection";
import { fillIdentityIntoPage, fillIdentityIntoScopeOf, hasIdentityForm } from "./identity-fill";

const alex: IdentityValues = {
  firstName: "Alex",
  middleName: "Q",
  lastName: "Example",
  email: "alex@example.com",
  phone: "+1 555 0100",
  birthDate: "1990-04-02",
  address: { street: "1 Main St", city: "Springfield", region: "IL", postalCode: "62704", country: "US" },
};

function getElement(selector: string): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

const getRect = () => ({ x: 0, y: 0, width: 200, height: 30, top: 0, left: 0, right: 200, bottom: 30 });

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("finding personal details", () => {
  it("finds names, email, phone and birth date by autocomplete, type and hints, never a username or company", () => {
    document.body.innerHTML = `
      <form>
        <input id="first" name="first_name">
        <input id="last" placeholder="Last name">
        <input id="email" type="email">
        <input id="phone" type="tel">
        <input id="birth" type="date" name="dob">
        <input id="username" name="username">
        <input id="company" name="company_name">
      </form>`;
    expect(findAllIdentityScope(document)[0].allField.map((field) => [field.element.id, field.kind])).toEqual([
      ["first", "given-name"],
      ["last", "family-name"],
      ["email", "email"],
      ["phone", "tel"],
      ["birth", "bday"],
    ]);
  });

  it("does not take a lone email box found by its type for a form to fill", () => {
    document.body.innerHTML = `<form><input type="email" name="newsletter"><button>Subscribe</button></form>`;
    expect(hasIdentityForm(document)).toBe(false);
  });

  it("offers identities on a checkout's name field and a sign-up's email, but not on a login's", () => {
    document.body.innerHTML = `
      <form id="checkout"><input id="name" autocomplete="name"><input id="email" autocomplete="email"><input id="street" autocomplete="street-address"><input id="city" autocomplete="address-level2"></form>
      <form id="sign-up"><input id="new-email" autocomplete="email"><input id="new-password" type="password" autocomplete="new-password"></form>
      <form id="login"><input id="login-email" autocomplete="email"><input id="login-password" type="password" autocomplete="current-password"></form>`;
    expect(findInlineMenuPurpose(document, getElement("#name"), getRect)).toBe("identity");
    expect(findInlineMenuPurpose(document, getElement("#city"), getRect)).toBe("address");
    expect(findInlineMenuPurpose(document, getElement("#new-email"), getRect)).toBe("identity");
    expect(findInlineMenuPurpose(document, getElement("#new-password"), getRect)).toBe("new-password");
    expect(findInlineMenuPurpose(document, getElement("#login-email"), getRect)).toBe("login");
  });
});

describe("filling an identity", () => {
  it("fills the empty personal fields and the address form beside them", () => {
    document.body.innerHTML = `
      <form>
        <input id="name" autocomplete="name">
        <input id="email" autocomplete="email" value="typed@example.com">
        <input id="phone" autocomplete="tel">
        <input id="birth" type="date" autocomplete="bday">
        <input id="street" autocomplete="street-address">
        <input id="city" autocomplete="address-level2">
        <input id="postal" autocomplete="postal-code">
      </form>
      <form><input id="other-name" autocomplete="name"><input id="other-email" autocomplete="email"></form>`;
    expect(fillIdentityIntoScopeOf(document, getElement("#name"), alex)).toBe(6);
    expect(getElement("#name").value).toBe("Alex Q Example");
    expect(getElement("#email").value).toBe("typed@example.com");
    expect(getElement("#phone").value).toBe("+1 555 0100");
    expect(getElement("#birth").value).toBe("1990-04-02");
    expect(getElement("#street").value).toBe("1 Main St");
    expect(getElement("#city").value).toBe("Springfield");
    expect(getElement("#postal").value).toBe("62704");
    expect(getElement("#other-name").value).toBe("");
    expect(document.activeElement).toBe(getElement("#name"));
  });

  it("splits the name across its fields, and leaves a date field empty for a birth date it cannot take", () => {
    document.body.innerHTML = `
      <form>
        <input id="first" autocomplete="given-name">
        <input id="middle" autocomplete="additional-name">
        <input id="last" autocomplete="family-name">
        <input id="birth" type="date" autocomplete="bday">
      </form>`;
    fillIdentityIntoPage(document, { ...alex, birthDate: "2 April 1990", address: undefined });
    expect(["#first", "#middle", "#last"].map((selector) => getElement(selector).value)).toEqual(["Alex", "Q", "Example"]);
    expect(getElement("#birth").value).toBe("");
  });
});
