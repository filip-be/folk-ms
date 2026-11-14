// Bal Folk Poznań – Site entry. Designers own the markup and styles; the data comes
// from the snapshots through contract/client (see sites/shared/). This Site has
// the Events pages and the Video Collection page (wideopomoc).
import { events } from "../../shared/render.ts";
import { run } from "../../shared/site.ts";
import { videos, videosPath } from "../../shared/videos.ts";

declare const __SNAPSHOT_URL__: string;
declare const __VIDEOS_URL__: string;

void run(document.getElementById("app")!, {
  modules: [
    events(__SNAPSHOT_URL__),
    videos(__VIDEOS_URL__, {
      intro: {
        pl: "Balfolkowa wideopomoc: filmy, które pomogą przypomnieć sobie kroki przed balem albo po warsztatach. Wybierz formę tańca lub kraj, z którego taniec pochodzi, albo wyszukaj go po nazwie.",
        en: "Balfolk video help: videos to help you recall the steps before a bal or after a workshop. Choose a dance form or the country the dance comes from, or search for it by name.",
      },
    }),
  ],
  nav: [
    { href: (lang) => `/${lang}/`, label: { pl: "Wydarzenia", en: "Events" } },
    { href: videosPath, label: { pl: "Wideopomoc", en: "Video help" } },
  ],
});
