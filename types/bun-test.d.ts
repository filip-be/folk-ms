// Minimal ambient types for the parts of Bun's runtime this repository uses
// (we avoid @types/bun to keep TypeScript the only npm package).
declare module "bun:test" {
  interface Matchers {
    toBe(v: unknown): void;
    toEqual(v: unknown): void;
    toBeUndefined(): void;
    toBeDefined(): void;
    toBeTruthy(): void;
    toBeFalsy(): void;
    toContain(v: unknown): void;
    toThrow(msg?: string | RegExp): void;
    toMatch(re: RegExp | string): void;
    toBeGreaterThan(n: number): void;
    not: Matchers;
  }
  export function describe(name: string, fn: () => void): void;
  export function test(name: string, fn: () => void | Promise<void>): void;
  export function expect(v: unknown): Matchers;
}
interface ImportMeta {
  dir: string;
}
declare module "node:fs" {
  export function readFileSync(path: string, enc: "utf8"): string;
  export function readdirSync(path: string): string[];
}
declare module "node:path" {
  export function join(...parts: string[]): string;
}

// The subset of the Bun runtime used by build scripts.
declare const Bun: {
  build(options: {
    entrypoints: string[];
    outdir?: string;
    minify?: boolean;
    target?: string;
    naming?: string;
    sourcemap?: string;
    define?: Record<string, string>;
  }): Promise<{ success: boolean; logs: unknown[] }>;
  file(path: string): Blob & { exists(): Promise<boolean> };
  write(path: string, data: string | Uint8Array): Promise<number>;
  serve(options: { port: number; fetch(req: Request): Response | Promise<Response> }): { port: number };
};
declare const process: { argv: string[]; exit(code: number): never; env: Record<string, string | undefined> };
declare module "node:fs" {
  export function copyFileSync(from: string, to: string): void;
  export function mkdirSync(path: string, opts?: { recursive?: boolean }): void;
}
