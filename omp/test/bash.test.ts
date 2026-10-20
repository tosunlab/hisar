import { describe, expect, test } from "bun:test";
import { classifyBashCommand, listHisarInvocations } from "../src/bash";

const blocked = [
  "hisar read hisar://Agents/Deploy%20token/password",
  "hisar get 'Deploy token' -f password",
  "hisar export -o /tmp/all.json",
  "hisar --standalone read hisar://Agents/Deploy%20token/password",
  "/Applications/Hisar.app/Contents/Helpers/hisar read hisar://Agents/Deploy%20token/password",
  "TOKEN=x hisar read hisar://Agents/Deploy%20token/password",
  "env -i PATH=/usr/bin hisar read hisar://Agents/Deploy%20token/password",
  "hisar read hisar://Agents/Deploy%20token/password | base64",
  "cd app && hisar get Deploy",
  "true; hisar export",
  "sh -c 'hisar read hisar://Agents/Deploy%20token/password'",
  'bash -lc "hisar get Deploy -f password"',
  "zsh -c 'echo start && hisar read hisar://Agents/Deploy%20token/password'",
  'TOKEN="$(hisar read hisar://Agents/Deploy%20token/password)" ./deploy.sh',
  'curl -H "Authorization: Bearer `hisar read hisar://Agents/API/credential`" https://example.com',
  "eval hisar read hisar://Agents/Deploy%20token/password",
  "hisar run --no-masking -- ./deploy.sh",
  "TOKEN=hisar://Agents/Deploy%20token/password hisar run --env-file .env --no-masking -- ./deploy.sh",
  "hisar run -no-masking=true -- env",
  "sh -c 'hisar run --no-masking -- env'",
];

const allowed = [
  "TOKEN=hisar://Agents/Deploy%20token/password hisar run -- ./deploy.sh",
  "hisar run --env-file .env -- npm test",
  "hisar run --env-file .env -- ./deploy.sh | tee deploy.log",
  "sh -c 'TOKEN=hisar://Agents/Deploy%20token/password hisar run -- ./deploy.sh'",
  "hisar run -- ./tool --no-masking",
  "hisar ls",
  "hisar status --json",
  "hisar vaults",
  'git commit -m "block hisar read in the plugin"',
  "rg 'hisar get' src",
  "echo hisar read # hisar read",
  "go test ./cmd/hisar -run TestRead",
  "ls ~/hisar-read",
];

describe("classifyBashCommand", () => {
  for (const command of blocked) {
    test(`blocks ${command}`, () => {
      const verdict = classifyBashCommand(command);
      expect(verdict.block).toBe(true);
      if (verdict.block) expect(verdict.reason).toContain("hisar run --");
    });
  }

  for (const command of allowed) {
    test(`allows ${command}`, () => {
      expect(classifyBashCommand(command)).toEqual({ block: false });
    });
  }

  test("names the subcommand it blocked", () => {
    const verdict = classifyBashCommand("hisar export");
    expect(verdict.block && verdict.reason).toStartWith("`hisar export`");
  });
});

describe("listHisarInvocations", () => {
  test("finds hisar commands for the status refresh", () => {
    expect(listHisarInvocations("TOKEN=hisar://A/B/c hisar run -- true")).toEqual([["run", "--", "true"]]);
    expect(listHisarInvocations("sh -c 'hisar ls' && hisar status")).toEqual([["ls"], ["status"]]);
    expect(listHisarInvocations("echo hisar")).toEqual([]);
  });
});
