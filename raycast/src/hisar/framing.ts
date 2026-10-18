import { HisarError } from "./errors";

/** The largest message either side may send (cliprotocol.MaxMessageSize). */
export const maximumMessageSize = 16 << 20;

const lengthSize = 4;

/** encodeFrame writes one message as cli.sock carries it: a 4-byte big-endian length, then the JSON. */
export function encodeFrame(message: unknown): Buffer {
  const body = Buffer.from(JSON.stringify(message), "utf8");
  if (body.length > maximumMessageSize) {
    throw new HisarError("invalid-request", "the request is larger than 16 MiB");
  }
  const frame = Buffer.alloc(lengthSize + body.length);
  frame.writeUInt32BE(body.length, 0);
  body.copy(frame, lengthSize);
  return frame;
}

/** FrameReader collects socket chunks and returns each complete message; a broken stream throws connection-lost. */
export class FrameReader {
  private buffered: Buffer = Buffer.alloc(0);

  push(chunk: Buffer): unknown[] {
    this.buffered = this.buffered.length === 0 ? chunk : Buffer.concat([this.buffered, chunk]);
    const allMessage: unknown[] = [];
    while (this.buffered.length >= lengthSize) {
      const length = this.buffered.readUInt32BE(0);
      if (length > maximumMessageSize) {
        throw new HisarError("connection-lost", "Hisar sent a message larger than 16 MiB");
      }
      if (this.buffered.length < lengthSize + length) break;
      const body = this.buffered.subarray(lengthSize, lengthSize + length);
      this.buffered = this.buffered.subarray(lengthSize + length);
      allMessage.push(parseJSON(body));
    }
    return allMessage;
  }
}

function parseJSON(body: Buffer): unknown {
  try {
    return JSON.parse(body.toString("utf8"));
  } catch {
    throw new HisarError("connection-lost", "Hisar sent a message that is not JSON");
  }
}
