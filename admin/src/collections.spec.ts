import { afterEach, describe, expect, it, vi } from "vitest";
import { readCollection } from "./collections";

const changed = "the collection changed during traversal; reload it before continuing";
const page = (key: string, nextCursor = "", snapshot = "snapshot") => new Response(JSON.stringify({ version: "1", snapshot, records: [{ key, field: "rows", value: { id: key } }], nextCursor }), { headers: { "Hisar-Collection-Pages": "1" } });
const read = () => readCollection<{ rows: { id: string }[] }>("/admin/fixture", () => ({ rows: [] }), (result, record) => result.rows.push(record.value as { id: string }));

describe("complete collection traversal", () => {
  afterEach(() => vi.restoreAllMocks());

  it("discards a stale partial attempt and consumes every page", async () => {
    let starts = 0;
    const fetch = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, options) => {
      expect(options?.headers).toMatchObject({ "Hisar-Collection-Pages": "1" });
      if (!String(input).includes("cursor=")) { starts++; return page("a", "next"); }
      if (starts === 1) return new Response(JSON.stringify({ error: changed }), { status: 409 });
      return page("b");
    });
    await expect(read()).resolves.toEqual({ rows: [{ id: "a" }, { id: "b" }] });
    expect(fetch).toHaveBeenCalledTimes(4);
  });

  it("bounds restarts under continuous mutation", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).includes("cursor=") ? new Response(JSON.stringify({ error: changed }), { status: 409 }) : page("a", "next"));
    await expect(read()).rejects.toThrow(changed);
    expect(fetch).toHaveBeenCalledTimes(6);
  });

  it("rejects mixed snapshots rather than publishing a partial list", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => String(input).includes("cursor=") ? page("b", "", "different") : page("a", "next"));
    await expect(read()).rejects.toThrow("snapshot");
  });

  it("keeps complete old-server payloads readable", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ rows: [{ id: "old" }] })));
    await expect(read()).resolves.toEqual({ rows: [{ id: "old" }] });
  });

  it("refuses claimed pages without a version", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ records: [] }), { headers: { "Hisar-Collection-Pages": "1" } }));
    await expect(read()).rejects.toThrow("invalid collection page");
  });

  it("stops reading a response above the page byte budget", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ version: "1", records: [], padding: "x".repeat(8 << 20) }), { headers: { "Hisar-Collection-Pages": "1" } }));
    await expect(read()).rejects.toThrow("byte budget");
  });
});
