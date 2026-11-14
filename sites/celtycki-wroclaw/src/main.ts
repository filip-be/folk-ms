// Celtycki Wrocław – Site entry. Designers own the markup and styles; the data comes
// from the snapshot through contract/client (see sites/shared/). This Site has
// the Events pages only.
import { events } from "../../shared/render.ts";
import { run } from "../../shared/site.ts";

declare const __SNAPSHOT_URL__: string;

void run(document.getElementById("app")!, { modules: [events(__SNAPSHOT_URL__)] });
