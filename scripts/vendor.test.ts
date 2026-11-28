// abcjs comes from npm (ADR 0012): the one runtime dependency, pinned exactly in
// package.json and with integrity in package-lock.json, and import()ed only by
// notation/abcjs.ts so it stays a lazy chunk. The vendored instrument samples
// (ADR 0012) are the reviewed ones, checked against their SHA256SUMS.
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

describe("abcjs from npm", () => {
  const pkg = JSON.parse(readFileSync(join(root, "package.json"), "utf8")) as { dependencies?: Record<string, string> };
  const lock = JSON.parse(readFileSync(join(root, "package-lock.json"), "utf8")) as {
    packages: Record<string, { version?: string; integrity?: string; dev?: boolean; dependencies?: Record<string, string> }>;
  };
  test("is the only runtime npm dependency, at an exact version", () => {
    expect(Object.keys(pkg.dependencies ?? {})).toEqual(["abcjs"]);
    expect(pkg.dependencies!.abcjs).toMatch(/^\d+\.\d+\.\d+$/);
    expect(existsSync(join(root, "bun.lock"))).toBe(false);
  });
  test("the lockfile pins that version with integrity, and nothing else ships", () => {
    const entry = lock.packages["node_modules/abcjs"];
    expect(entry?.version).toBe(pkg.dependencies!.abcjs);
    expect(entry?.integrity).toMatch(/^sha512-/);
    expect(entry?.dependencies ?? {}).toEqual({});
    const runtime = Object.entries(lock.packages).filter(([name, p]) => name !== "" && !p.dev).map(([name]) => name);
    expect(runtime).toEqual(["node_modules/abcjs"]);
  });
  test("the wrapper import()s the prebuilt dist file, so it stays one lazy chunk", () => {
    const wrapper = readFileSync(join(root, "notation", "abcjs.ts"), "utf8");
    expect(wrapper).toContain('import("abcjs/dist/abcjs-basic-min.js")');
    // Only type imports of the package itself: a value import would bundle it into the entry.
    expect(wrapper).not.toMatch(/^import (?!type )[^;]*from "abcjs/m);
    for (const f of ["cms/admin/src", "sites"]) {
      for (const file of readdirSync(join(root, f), { recursive: true, encoding: "utf8" })) {
        if (!file.endsWith(".ts") || file.includes("node_modules")) continue;
        expect(`${f}/${file} ${/from "abcjs[/"]|import\("abcjs/.test(readFileSync(join(root, f, file), "utf8"))}`).toBe(`${f}/${file} false`);
      }
    }
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
