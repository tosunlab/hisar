import raycastConfig from "@raycast/eslint-config";
import { defineConfig } from "eslint/config";

const outsideRaycast = "src/hisar is the cli.sock client and must run outside Raycast.";

export default defineConfig([
  { ignores: ["dist/", "raycast-env.d.ts"] },
  ...raycastConfig,
  {
    files: ["src/hisar/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: [
            { name: "@raycast/api", message: outsideRaycast },
            { name: "@raycast/utils", message: outsideRaycast },
            { name: "react", message: outsideRaycast },
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
