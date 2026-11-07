// Bal Folk Poznań – Site entry. Designers own the markup and styles; the data comes
// from the snapshot through contract/client (see sites/shared/render.ts).
import { start } from "../../shared/render.ts";

declare const __SNAPSHOT_URL__: string;

void start(document.getElementById("app")!, __SNAPSHOT_URL__);
