import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync } from "node:zlib";

const root = fileURLToPath(new URL("../", import.meta.url));
const size = 32;

function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit += 1) {
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(type, data) {
  const name = Buffer.from(type, "ascii");
  const output = Buffer.alloc(12 + data.length);
  output.writeUInt32BE(data.length, 0);
  name.copy(output, 4);
  data.copy(output, 8);
  output.writeUInt32BE(crc32(Buffer.concat([name, data])), 8 + data.length);
  return output;
}

const scanlines = Buffer.alloc(size * (1 + size * 4));
for (let y = 0; y < size; y += 1) {
  const row = y * (1 + size * 4);
  for (let x = 0; x < size; x += 1) {
    const pixel = row + 1 + x * 4;
    const mark = (y >= 8 && y <= 12 && x >= 7 && x <= 24) ||
      (x >= 14 && x <= 18 && y >= 12 && y <= 25);
    scanlines[pixel] = mark ? 255 : 37;
    scanlines[pixel + 1] = mark ? 255 : 99;
    scanlines[pixel + 2] = mark ? 255 : 235;
    scanlines[pixel + 3] = 255;
  }
}

const header = Buffer.alloc(13);
header.writeUInt32BE(size, 0);
header.writeUInt32BE(size, 4);
header[8] = 8;
header[9] = 6;

const png = Buffer.concat([
  Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
  chunk("IHDR", header),
  chunk("IDAT", deflateSync(scanlines)),
  chunk("IEND", Buffer.alloc(0)),
]);
const iconDirectory = join(root, "apps", "desktop", "src-tauri", "icons");
mkdirSync(iconDirectory, { recursive: true });
writeFileSync(join(iconDirectory, "icon.png"), png);
