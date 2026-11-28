// Celtycki Wrocław – Site entry. Designers own the markup and styles; the data comes
// from the snapshots through contract/client (see sites/shared/). This Site has
// the Events pages and the Tune Collection pages (tunes, sets).
import { events } from "../../shared/render.ts";
import { run } from "../../shared/site.ts";
import { tunes, tunesPath } from "../../shared/tunes.ts";

declare const __SNAPSHOT_URL__: string;
declare const __TUNES_BASE__: string;

void run(document.getElementById("app")!, {
  // tunes() first: it stops playback whenever the address changes.
  modules: [tunes(__TUNES_BASE__), events(__SNAPSHOT_URL__)],
  nav: [
    { href: (lang) => `/${lang}/`, label: { pl: "Wydarzenia", en: "Events" } },
    { href: tunesPath, label: { pl: "Melodie", en: "Tunes" } },
  ],
});
