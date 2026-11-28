// Downloads the instrument samples into vendor/soundfonts/FluidR3_GM and writes
// their SHA256SUMS: bun scripts/fetch-samples.ts. Run it only to change the
// instruments or the note range (scripts/samples.ts); the files are committed.
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { SOUNDFONT_DIR, SOURCE, samplePaths } from "./samples.ts";

const sums: string[] = [];
for (const p of samplePaths()) {
  const res = await fetch(`${SOURCE}/${p}`);
  if (!res.ok) throw new Error(`${p}: HTTP ${res.status}`);
  const data = new Uint8Array(await res.arrayBuffer());
  // An MP3 starts with an ID3 tag or an MPEG frame sync.
  if (!(data[0] === 0x49 && data[1] === 0x44 && data[2] === 0x33) && !(data[0] === 0xff && (data[1]! & 0xe0) === 0xe0)) throw new Error(`${p}: not an MP3`);
  mkdirSync(dirname(join(SOUNDFONT_DIR, p)), { recursive: true });
  writeFileSync(join(SOUNDFONT_DIR, p), data);
  sums.push(`${new Bun.CryptoHasher("sha256").update(data).digest("hex")}  ${p}`);
}
writeFileSync(join(SOUNDFONT_DIR, "SHA256SUMS"), sums.join("\n") + "\n");
console.log(`${sums.length} samples in ${SOUNDFONT_DIR}`);
