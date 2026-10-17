// make build-admin copies Go's wasm_exec.js into admin/wasm; importing it only defines globalThis.Go.
declare module "*/wasm_exec.js";
