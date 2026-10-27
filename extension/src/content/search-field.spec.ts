import { beforeEach, describe, expect, it } from "vitest";
import { isSearchField } from "./search-field";

function loadPage(html: string): void {
  document.body.innerHTML = html;
}

function getInput(selector: string): HTMLInputElement {
  return document.querySelector<HTMLInputElement>(selector)!;
}

beforeEach(() => {
  document.body.innerHTML = "";
});

describe("isSearchField", () => {
  it("knows a search box by its type, role, list, keyboard or landmark, with no words at all", () => {
    loadPage(`
      <input id="by-type" type="search">
      <input id="by-role" type="text" role="searchbox">
      <input id="by-combobox" type="text" role="combobox" aria-autocomplete="list">
      <input id="by-typeahead" type="text" aria-autocomplete="both">
      <input id="by-list" type="text" list="options"><datalist id="options"></datalist>
      <input id="by-keyboard" type="text" enterkeyhint="search">
      <input id="by-inputmode" type="text" inputmode="search">
      <div role="search"><input id="in-landmark" type="text"></div>
      <search><input id="in-search-element" type="text"></search>
      <form action="/search"><input id="in-search-form" type="text" name="text"></form>
    `);
    for (const id of ["by-type", "by-role", "by-combobox", "by-typeahead", "by-list", "by-keyboard", "by-inputmode", "in-landmark", "in-search-element", "in-search-form"]) {
      expect(isSearchField(getInput(`#${id}`)), id).toBe(true);
    }
  });

  it("knows a search box by its name or its words, in several languages", () => {
    loadPage(`
      <input id="q" type="text" name="q">
      <input id="s" type="text" name="s">
      <input id="search-query" type="text" name="search_query">
      <input id="keyword" type="text" placeholder="Keyword">
      <input id="aria" type="text" aria-label="Search Amazon">
      <input id="turkish" type="text" placeholder="Ürün, kategori veya marka ara">
      <input id="turkish-2" type="text" placeholder="Arama">
      <input id="german" type="text" placeholder="Suche">
      <input id="french" type="text" placeholder="Rechercher">
      <input id="spanish" type="text" placeholder="Buscar">
      <input id="filter" type="text" placeholder="Filter rows">
      <label for="labelled">Search the docs</label><input id="labelled" type="text">
    `);
    for (const id of ["q", "s", "search-query", "keyword", "aria", "turkish", "turkish-2", "german", "french", "spanish", "filter", "labelled"]) {
      expect(isSearchField(getInput(`#${id}`)), id).toBe(true);
    }
  });

  it("leaves the fields of a sign-in alone, and any field that is not a text box", () => {
    loadPage(`
      <form action="/session">
        <input id="username" type="text" name="login" autocomplete="username">
        <input id="email" type="email" name="email">
        <input id="password" type="password" name="password">
        <input id="find-account" type="text" aria-label="Find your account">
        <input id="research" type="text" name="given-name" placeholder="First name">
      </form>
    `);
    for (const id of ["username", "email", "password", "find-account", "research"]) {
      expect(isSearchField(getInput(`#${id}`)), id).toBe(false);
    }
    expect(isSearchField(document.createElement("textarea"))).toBe(false);
  });
});
