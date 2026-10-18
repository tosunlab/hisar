import { describe, expect, it } from "vitest";
import { encodeFrame, FrameReader, maximumMessageSize } from "./framing";

describe("cli.sock framing", () => {
  it("reads back messages split across chunks and several messages in one chunk", () => {
    const first = { id: "1", type: "hello", clientPlatform: "darwin/raycast" };
    const second = { id: "2", type: "launcher.items.password", vaultID: "v", itemID: "ü" };
    const stream = Buffer.concat([encodeFrame(first), encodeFrame(second)]);
    const reader = new FrameReader();

    const allMessage = [...reader.push(stream.subarray(0, 3)), ...reader.push(stream.subarray(3, 20))];
    allMessage.push(...reader.push(stream.subarray(20)));

    expect(allMessage).toEqual([first, second]);
  });

  it("writes a 4-byte big-endian length before the JSON", () => {
    const frame = encodeFrame({ id: "1" });
    expect(frame.readUInt32BE(0)).toBe(Buffer.byteLength('{"id":"1"}'));
    expect(frame.subarray(4).toString("utf8")).toBe('{"id":"1"}');
  });

  it("accepts a 16 MiB message and refuses a larger one either way", () => {
    const largest = "x".repeat(maximumMessageSize - 2);
    expect(new FrameReader().push(encodeFrame(largest))).toEqual([largest]);
    expect(() => encodeFrame(`${largest}x`)).toThrow(expect.objectContaining({ code: "invalid-request" }));

    const oversizedHeader = Buffer.alloc(4);
    oversizedHeader.writeUInt32BE(maximumMessageSize + 1, 0);
    expect(() => new FrameReader().push(oversizedHeader)).toThrow(expect.objectContaining({ code: "connection-lost" }));
  });
});
