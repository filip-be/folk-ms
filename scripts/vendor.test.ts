// The vendored files of ADR 0012 are the reviewed ones: abcjs at its exact
// version and the instrument samples, each checked against its SHA256SUMS; and
// abcjs is not an npm dependency.
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { INSTRUMENTS, SOUNDFONT_DIR, noteName, samplePaths } from "./samples.ts";

const root = join(import.meta.dir, "..");

function sums(dir: string): Map<string, string> {
  const m = new Map<string, string>();
  for (const line of readFileSync(join(dir, "SHA256SUMS"), "utf8").split("\n").filter(Boolean)) {
    const [hash, name] = line.split(/ {2}/);
    m.set(name!, hash!);
  }
  return m;
}

function sha256(path: string): string {
  return new Bun.CryptoHasher("sha256").update(readFileSync(path)).digest("hex");
}

describe("vendored abcjs", () => {
  const dir = join(root, "vendor", "abcjs", "6.7.3");
  test("files match SHA256SUMS", () => {
    const want = sums(dir);
    expect([...want.keys()].sort()).toEqual(["LICENSE", "abcjs-basic-min.js", "abcjs-basic-min.js.LICENSE"]);
    for (const [name, hash] of want) expect(`${name} ${sha256(join(dir, name))}`).toBe(`${name} ${hash}`);
  });
  test("is the release the wrapper names", () => {
    expect(readFileSync(join(dir, "abcjs-basic-min.js"), "utf8").slice(0, 200)).toContain("abcjs_basic v6.7.3");
    expect(readFileSync(join(root, "vendor", "abcjs", "abcjs.ts"), "utf8")).toContain('import("./6.7.3/abcjs-basic-min.js")');
  });
  test("is not an npm dependency", () => {
    expect(readFileSync(join(root, "package.json"), "utf8")).not.toContain("abcjs");
    expect(readFileSync(join(root, "package-lock.json"), "utf8")).not.toContain("abcjs");
    expect(existsSync(join(root, "bun.lock"))).toBe(false);
  });
});

describe("vendored instrument samples", () => {
  test("note names are the ones abcjs asks for", () => {
    expect([noteName(55), noteName(61), noteName(69), noteName(70), noteName(98)]).toEqual(["G3", "Db4", "A4", "Bb4", "D7"]);
  });
  test("every sample is present and matches SHA256SUMS, and nothing else is", () => {
    const want = sums(SOUNDFONT_DIR);
    expect([...want.keys()].sort()).toEqual(samplePaths().sort());
    for (const [name, hash] of want) expect(`${name} ${sha256(join(SOUNDFONT_DIR, name))}`).toBe(`${name} ${hash}`);
    const onDisk = INSTRUMENTS.flatMap((i) => readdirSync(join(SOUNDFONT_DIR, `${i}-mp3`)).map((f) => `${i}-mp3/${f}`));
    expect(onDisk.sort()).toEqual(samplePaths().sort());
    expect(readdirSync(SOUNDFONT_DIR).sort()).toEqual(["CREDITS", "SHA256SUMS", ...INSTRUMENTS.map((i) => `${i}-mp3`)].sort());
  });
  test("the credit names the soundfont and its licence", () => {
    const credits = readFileSync(join(SOUNDFONT_DIR, "CREDITS"), "utf8");
    expect(credits).toContain("Frank Wen");
    expect(credits).toContain("CC BY 3.0");
  });
});
