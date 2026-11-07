// Serves a built Site locally with the same fallback as SWA's
// navigationFallback: bun sites/serve.ts <site-id> <port>
import { join } from "node:path";

const [site, port] = [process.argv[2], Number(process.argv[3] ?? 5173)];
if (!site || !/^[a-z0-9-]+$/.test(site)) {
  console.error("usage: bun sites/serve.ts <site-id> [port]");
  process.exit(2);
}
const dist = join(import.meta.dir, site, "dist");
Bun.serve({
  port,
  async fetch(req) {
    const path = new URL(req.url).pathname;
    if (!path.includes("..") && /\.[a-z0-9]+$/.test(path)) {
      const f = Bun.file(join(dist, path));
      if (await f.exists()) return new Response(f);
      return new Response("not found", { status: 404 });
    }
    return new Response(Bun.file(join(dist, "index.html")), { headers: { "Content-Type": "text/html; charset=utf-8" } });
  },
});
console.log(`${site} on http://localhost:${port}/`);
