// Routes every Site supports. The event route must match the Permalink
// pattern the CMS writes into snapshots and Captions (config/sites.json
// permalinkTemplate); router.test.ts checks it against that file.

import type { Lang } from "../../contract/client/src/index.ts";

export type Route =
  | { name: "home"; lang: Lang }
  | { name: "events"; lang: Lang; past: boolean }
  | { name: "event"; lang: Lang; shortId: string; slug: string }
  | { name: "notFound"; lang: Lang };

const LANGS: Lang[] = ["pl", "en"];

/** Resolves a pathname into a Route. "/" and unknown languages go to Polish. */
export function match(pathname: string): Route {
  const parts = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  const lang = (LANGS as string[]).includes(parts[0] ?? "") ? (parts[0] as Lang) : "pl";
  if (parts.length === 0 || (parts.length === 1 && parts[0] === lang)) return { name: "home", lang };
  if (parts[0] !== lang) return { name: "notFound", lang };
  if (parts[1] === "events" && parts.length === 2) return { name: "events", lang, past: false };
  if (parts[1] === "events" && parts[2] === "past" && parts.length === 3) return { name: "events", lang, past: true };
  if (parts[1] === "events" && parts.length === 3) {
    const m = /^([a-z2-7]{8})(?:-(.*))?$/.exec(parts[2]!);
    if (m) return { name: "event", lang, shortId: m[1]!, slug: m[2] ?? "" };
  }
  return { name: "notFound", lang };
}

/** Builds the path of a route (the inverse of match). */
export function href(route: Route): string {
  switch (route.name) {
    case "home":
      return `/${route.lang}/`;
    case "events":
      return `/${route.lang}/events${route.past ? "/past" : ""}`;
    case "event":
      return `/${route.lang}/events/${route.shortId}${route.slug ? "-" + route.slug : ""}`;
    case "notFound":
      return `/${route.lang}/`;
  }
}
