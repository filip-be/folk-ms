// Harness of the CMS's dev-only practice page (/dev/facebook/events/create/),
// which mimics the roles and labels of Facebook's create-event form: it
// runs the fill engine with a sample Handoff and shows what happened, for
// regression of the engine without Facebook.

import { parseFieldMap } from "./fieldmap.ts";
import { ensureFocus, fillAll, findForm, waitFor } from "./fill.ts";
import type { Handoff } from "./protocol.ts";

const sample = (fieldMap: unknown): Handoff => ({
  id: "practice",
  org: "practice",
  eventId: "practice",
  occurrenceKey: "2026-12-20",
  token: "",
  pageName: "Folk MS",
  lang: "en",
  returnUrl: location.origin + "/app/",
  fieldMap,
  fields: {
    name: "Bal grudniowy",
    startDate: "2026-12-20",
    startTime: "19:00",
    endDate: "2026-12-21",
    endTime: "01:00",
    timezone: "Europe/Warsaw",
    location: "Dom Kultury Zamek, ul. Św. Marcin 80/82, Poznań",
    description: "Zapraszamy na bal.\n\nSzczegóły: https://example.org/pl/events/abcdefgh-bal",
    ticketUrl: "https://tickets.example/bal",
  },
  cover: { name: "cover.jpg", type: "image/jpeg", data: "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AKp//2Q==" },
});

async function run(): Promise<void> {
  const out = document.getElementById("report")!;
  out.textContent = "running…";
  const map = parseFieldMap(await (await fetch("../../fieldmap.json")).json());
  const found = await waitFor(() => findForm(map), 5000);
  if (!found) {
    out.textContent = JSON.stringify({ error: "form not found" });
    return;
  }
  await ensureFocus(() => (out.textContent = "click anywhere on this page to continue"));
  const report = await fillAll(map, sample(map), found, () => undefined);
  out.textContent = JSON.stringify(report, null, 2);
  out.dataset.done = "true";
}

document.getElementById("run")?.addEventListener("click", () => void run());
