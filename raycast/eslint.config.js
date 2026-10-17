const { defineConfig } = require("eslint/config");
const raycastConfig = require("@raycast/eslint-config");

module.exports = defineConfig([
  ...raycastConfig,
  {
    files: ["src/hisar/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: [
            { name: "@raycast/api", message: "src/hisar is the cli.sock client and must run outside Raycast." },
            { name: "@raycast/utils", message: "src/hisar is the cli.sock client and must run outside Raycast." },
            { name: "react", message: "src/hisar is the cli.sock client and must run outside Raycast." },
          ],
        },
      ],
    },
  },
  {
    files: ["src/*.{ts,tsx}", "src/raycast/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        { patterns: [{ group: ["**/hisar/*"], message: "Import the client's public surface, ./hisar, only." }] },
      ],
    },
  },
]);
