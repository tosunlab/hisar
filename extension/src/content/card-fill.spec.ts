import { beforeEach, describe, expect, it } from "vitest";
import type { CardValues } from "../shared/protocol";
import { classifyCardField, findAllCardScope } from "./card-detection";
import { fillCardIntoPage, fillCardIntoScopeOf, hasCardForm, isFocusedCardField } from "./card-fill";

const visa: CardValues = { cardholder: "Alex Example", number: "4111111111111111", expiryMonth: "03", expiryYear: "2030", securityCode: "123", brand: "Visa" };

function getElement<Element extends HTMLElement = HTMLInputElement>(selector: string): Element {
  const element = document.querySelector<Element>(selector);
  if (!element) throw new Error(`missing ${selector}`);
  return element;
}

function getValue(selector: string): string {
  return getElement<HTMLInputElement | HTMLSelectElement>(selector).value;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("finding a payment form", () => {
  it("finds the fields by their autocomplete tokens", () => {
    document.body.innerHTML = `
      <form id="pay">
        <input id="name" autocomplete="cc-name">
        <input id="number" autocomplete="cc-number">
        <input id="expiry" autocomplete="cc-exp">
        <input id="code" autocomplete="cc-csc">
      </form>`;
    const [scope] = findAllCardScope(document);
    expect(scope.allField.map((field) => [field.element.id, field.kind])).toEqual([
      ["name", "name"],
      ["number", "number"],
      ["expiry", "expiry"],
      ["code", "security-code"],
    ]);
  });

  it("finds them by their names and labels, a combined expiry before its month and year", () => {
    document.body.innerHTML = `
      <div>
        <label for="a">Card number</label><input id="a">
        <input id="b" placeholder="MM / YY">
        <input id="c" name="cvc">
        <input id="d" aria-label="Name on card">
      </div>`;
    expect(findAllCardScope(document)[0].allField.map((field) => field.kind)).toEqual(["number", "expiry", "security-code", "name"]);
  });

  it("needs a card number, so a birth month or a lone name is not a payment form", () => {
    document.body.innerHTML = `
      <form><input name="birth_month"><input name="year"><input aria-label="Cardholder"></form>
      <form><input name="email" autocomplete="email"><input name="search" placeholder="Search cards"></form>`;
    expect(findAllCardScope(document)).toEqual([]);
    expect(hasCardForm(document)).toBe(false);
    expect(classifyCardField(getElement("[name=email]"))).toBeUndefined();
  });

  it("leaves out a birth month or year in other languages, beside a card number", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" name="cardnumber">
        <label>Geburtsmonat <select id="de-month"><option value="1">1</option><option value="3">3</option></select></label>
        <label>Mes de nacimiento <input id="es-month"></label>
        <label>Mois de naissance <input id="fr-month"></label>
        <label>Doğum ayı <input id="tr-month"></label>
        <label>Anno di nascita <input id="it-year"></label>
        <input id="month" name="exp_month">
      </form>`;
    expect(findAllCardScope(document)[0].allField.map((field) => field.element.id)).toEqual(["number", "month"]);
  });

  it("leaves out hidden fields, so a page cannot collect a card the user does not see", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" autocomplete="cc-number">
        <input id="hidden-code" autocomplete="cc-csc" style="display: none">
        <input id="transparent-name" autocomplete="cc-name" style="opacity: 0">
      </form>`;
    expect(findAllCardScope(document)[0].allField.map((field) => field.element.id)).toEqual(["number"]);
  });
});

describe("filling a card", () => {
  it("fills the anchored form's empty fields with the expiry as the field asks", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" autocomplete="cc-number">
        <input id="name" autocomplete="cc-name" value="Typed Name">
        <input id="expiry" autocomplete="cc-exp" placeholder="MM / YY">
        <input id="code" autocomplete="cc-csc" type="password">
      </form>
      <form><input id="other" autocomplete="cc-number"></form>`;
    const allEvent: string[] = [];
    document.addEventListener("input", (event) => allEvent.push((event.target as HTMLElement).id));
    expect(fillCardIntoScopeOf(document, getElement("#number"), visa)).toBe(3);
    expect(getValue("#number")).toBe("4111111111111111");
    expect(getValue("#name")).toBe("Typed Name");
    expect(getValue("#expiry")).toBe("03 / 30");
    expect(getValue("#code")).toBe("123");
    expect(getValue("#other")).toBe("");
    expect(allEvent).toEqual(["number", "expiry", "code"]);
    expect(document.activeElement).toBe(getElement("#number"));
  });

  it("writes four digits of year where the field asks, and fills a month input", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" autocomplete="cc-number">
        <input id="long" autocomplete="cc-exp" placeholder="MM/YYYY">
      </form>
      <form>
        <input id="number2" autocomplete="cc-number">
        <input id="month" type="month" autocomplete="cc-exp">
        <input id="short" autocomplete="cc-exp-year" maxlength="2">
      </form>`;
    fillCardIntoScopeOf(document, getElement("#number"), visa);
    expect(getValue("#long")).toBe("03/2030");
    fillCardIntoScopeOf(document, getElement("#number2"), visa);
    expect(getValue("#month")).toBe("2030-03");
    expect(getValue("#short")).toBe("30");
  });

  it("chooses the month and year in lists by value, text or name", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" name="cardnumber">
        <select id="month" name="exp_month"><option value="">Month</option><option value="1">January</option><option value="3">March</option></select>
        <select id="year" name="exp_year"><option value="">Year</option><option value="29">29</option><option value="30">30</option></select>
        <select id="named-month" name="card_month"><option>Jan</option><option>Mar</option></select>
      </form>`;
    expect(fillCardIntoPage(document, visa)).toBe(4);
    expect(getValue("#month")).toBe("3");
    expect(getValue("#year")).toBe("30");
    expect(getValue("#named-month")).toBe("Mar");
  });

  it("chooses a month by its whole name in other languages", () => {
    const allMonthList = (id: string, allName: string[]) => `<select id="${id}" name="exp_month_${id}">${allName.map((name) => `<option>${name}</option>`).join("")}</select>`;
    const fillMonth = (expiryMonth: string) => {
      document.body.innerHTML = `
        <form>
          <input id="number" name="cardnumber">
          ${allMonthList("es", ["Marzo", "Abril", "Mayo", "Junio", "Julio"])}
          ${allMonthList("pt", ["Março", "Abril", "Maio", "Junho", "Julho"])}
          ${allMonthList("it", ["Marzo", "Aprile", "Maggio", "Giugno", "Luglio"])}
          ${allMonthList("fr", ["Mars", "Avril", "Mai", "Juin", "Juillet"])}
          ${allMonthList("fr-short", ["mars", "avr.", "mai", "juin", "juil."])}
          ${allMonthList("de", ["März", "April", "Mai", "Juni", "Juli"])}
        </form>`;
      fillCardIntoPage(document, { ...visa, expiryMonth });
      return ["es", "pt", "it", "fr", "fr-short", "de"].map((id) => getValue(`#${id}`));
    };
    expect(fillMonth("05")).toEqual(["Mayo", "Maio", "Maggio", "Mai", "mai", "Mai"]);
    expect(fillMonth("07")).toEqual(["Julio", "Julho", "Luglio", "Juillet", "juil.", "Juli"]);
    expect(fillMonth("03")).toEqual(["Marzo", "Março", "Marzo", "Mars", "mars", "März"]);
  });

  it("keeps a month or year the list already has", () => {
    document.body.innerHTML = `
      <form>
        <input id="number" name="cardnumber">
        <select id="chosen" name="exp_month"><option value="">Month</option><option value="1">01</option><option value="3">03</option></select>
        <select id="preset" name="exp_year"><option value="2029">2029</option><option value="2030">2030</option><option value="2031" selected>2031</option></select>
      </form>`;
    getElement<HTMLSelectElement>("#chosen").value = "1";
    expect(fillCardIntoPage(document, visa)).toBe(1);
    expect(getValue("#chosen")).toBe("1");
    expect(getValue("#preset")).toBe("2031");
  });

  it("splits the number across boxes", () => {
    document.body.innerHTML = `
      <form>
        <input id="n1" name="card_number_1" maxlength="4">
        <input id="n2" name="card_number_2" maxlength="4">
        <input id="n3" name="card_number_3" maxlength="4">
        <input id="n4" name="card_number_4" maxlength="4">
      </form>`;
    fillCardIntoScopeOf(document, getElement("#n1"), visa);
    expect(["#n1", "#n2", "#n3", "#n4"].map(getValue)).toEqual(["4111", "1111", "1111", "1111"]);
  });

  it("fills the focused form, and says when a payment field has the focus", () => {
    document.body.innerHTML = `
      <form><input id="first" autocomplete="cc-number"></form>
      <form><input id="second" autocomplete="cc-number"></form>`;
    getElement("#second").focus();
    expect(isFocusedCardField(document)).toBe(true);
    fillCardIntoPage(document, visa);
    expect(getValue("#first")).toBe("");
    expect(getValue("#second")).toBe("4111111111111111");
  });
});
