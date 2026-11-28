// Serves a built Site locally with the same fallback as SWA's
// navigationFallback and the same globalHeaders (so the CSP applies):
// bun sites/serve.ts <site-id> <port>
import { readFileSync } from "node:fs";
import { join } from "node:path";

const [site, port] = [process.argv[2], Number(process.argv[3] ?? 5173)];
if (!site || !/^[a-z0-9-]+$/.test(site)) {
  console.error("usage: bun sites/serve.ts <site-id> [port]");
  process.exit(2);
}
const dist = join(import.meta.dir, site, "dist");
const config = JSON.parse(readFileSync(join(dist, "staticwebapp.config.json"), "utf8")) as { globalHeaders?: Record<string, string> };
const headers = config.globalHeaders ?? {};
Bun.serve({
  port,
  async fetch(req) {
    const path = new URL(req.url).pathname;
    if (!path.includes("..") && /\.[a-z0-9]+$/.test(path)) {
      const f = Bun.file(join(dist, path));
      if (await f.exists()) return new Response(f, { headers });
      return new Response("not found", { status: 404 });
    }
    return new Response(Bun.file(join(dist, "index.html")), { headers: { ...headers, "Content-Type": "text/html; charset=utf-8" } });
  },
});
console.log(`${site} on http://localhost:${port}/`);
