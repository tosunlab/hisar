// Node needs the .js file names to load selenium-webdriver's CommonJS modules; their types are published without them.
declare module "selenium-webdriver/firefox.js" {
  export * from "selenium-webdriver/firefox";
}

declare module "selenium-webdriver/lib/command.js" {
  export * from "selenium-webdriver/lib/command";
}
