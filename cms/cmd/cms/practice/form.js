// Mimics the behaviour Autofill relies on in Facebook's create-event form
// (spike of 2026-11-10, ADR 0006): labelled controls, dates that commit on
// blur and silently revert when the text cannot be parsed, comboboxes whose
// innermost role=option takes the click, controls revealed by buttons, and a
// client-side replaceState to /events/{id}/ on Create.
(() => {
  const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  const d = document.getElementById("dialog");
  const el = (tag, attrs = {}, ...kids) => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "text") e.textContent = v;
      else e.setAttribute(k, v);
    }
    e.append(...kids);
    return e;
  };
  const fmt = (dt) => `${dt.getDate()} ${MONTHS[dt.getMonth()]} ${dt.getFullYear()}`;
  function parseDate(s) {
    let m = /^(\d{1,2}) ([A-Za-z]{3}) (\d{4})$/.exec(s.trim());
    if (m && MONTHS.includes(m[2])) return new Date(+m[3], MONTHS.indexOf(m[2]), +m[1]);
    m = /^(\d{1,2})[/.](\d{1,2})[/.](\d{4})$/.exec(s.trim());
    if (m && +m[2] >= 1 && +m[2] <= 12) return new Date(+m[3], +m[2] - 1, +m[1]);
    return null;
  }

  function listbox(anchor, items, onPick) {
    document.querySelectorAll("ul[role=listbox]").forEach((l) => l.remove());
    const ul = el("ul", { role: "listbox" });
    for (const text of items) {
      const inner = el("div", { role: "option", "aria-selected": "false" }, text);
      inner.addEventListener("click", () => {
        ul.remove();
        onPick(text);
      });
      ul.append(el("li", { role: "option", "aria-selected": "false" }, inner)); // LI clicks do nothing
    }
    document.body.append(ul);
    anchor.setAttribute("aria-expanded", "true");
  }

  function dateField(name) {
    let value = new Date(2026, 10, 10);
    const btn = el("div", { role: "button", "aria-label": `${name}, selected: ${fmt(value)}` });
    const input = el("input", { type: "text", role: "combobox" });
    input.value = fmt(value);
    input.addEventListener("blur", () => {
      const p = parseDate(input.value);
      if (p) value = p;
      input.value = fmt(value);
      btn.setAttribute("aria-label", `${name}, selected: ${fmt(value)}`);
    });
    return el("label", {}, el("span", { text: name }), btn, input);
  }
  function timeField(name, initial) {
    let value = initial;
    const input = el("input", { type: "text", role: "combobox" });
    input.value = value;
    input.addEventListener("blur", () => {
      if (/^([01]\d|2[0-3]):[0-5]\d$/.test(input.value.trim())) value = input.value.trim();
      input.value = value;
    });
    return el("label", {}, el("span", { text: name }), input);
  }
  function tzField() {
    const input = el("input", { type: "search", role: "combobox", "aria-label": "Time zone" });
    input.value = "GMT+1";
    input.addEventListener("input", () => {
      const zones = ["Europe/Warsaw\nWarsaw Time (UTC+01)", "Europe/London\nLondon Time (UTC+00)"].filter((z) => z.toLowerCase().includes(input.value.toLowerCase()));
      listbox(input, zones, (z) => (input.value = z.includes("UTC+01") ? "UTC+01" : "UTC+00"));
    });
    return el("label", {}, el("span", { text: "Time zone" }), input);
  }

  const file = el("input", { type: "file", accept: "image/*", class: "hidden" });
  const cover = el("div", { role: "button", "aria-label": "Add cover photo to event", text: "Add" });
  cover.addEventListener("click", () => file.click());
  file.addEventListener("change", () => cover.setAttribute("aria-label", "Edit event cover photo or video"));
  const host = el("div", { role: "button", "aria-label": "Folk MS, Host – Your profile, select event host", text: "Folk MS" });
  const name = el("label", {}, el("span", { text: "Event name" }), el("input", { type: "text" }));

  const end = el("div", { class: "hidden" }, dateField("End date"), timeField("End time", "22:00"), tzField());
  const addEnd = el("div", { role: "button", "aria-label": "Add end date and time", text: "+ End date and time" });
  addEnd.addEventListener("click", () => {
    end.classList.remove("hidden");
    addEnd.remove();
  });

  const loc = el("label", { class: "hidden" }, el("span", { text: "Add location" }), el("input", { type: "search", role: "combobox" }));
  const locInput = loc.querySelector("input");
  locInput.addEventListener("input", () => {
    const t = locInput.value;
    listbox(locInput, ["Some Place\nul. Elsewhere 1 · Poznan", `Just use "${t}"`], (o) => {
      if (o.startsWith("Some Place")) locInput.value = "Some Place";
    });
  });
  const mode = el("label", { role: "combobox" }, el("span", { text: "Is it in person or virtual?" }), el("span", { text: " " }));
  mode.addEventListener("click", () =>
    listbox(mode, ["In person", "Virtual"], (o) => {
      mode.lastChild.textContent = o;
      loc.classList.toggle("hidden", o !== "In person");
    }),
  );

  const details = el("label", {}, el("span", { text: "What are the details?" }), el("textarea", { rows: "4" }));
  const ticket = el("label", { class: "hidden" }, el("span", { text: "Ticket URL" }), el("input", { type: "text" }));
  const addTicket = el("div", { role: "button", text: "Add tickets" });
  addTicket.addEventListener("click", () => ticket.classList.remove("hidden"));

  const create = el("div", { role: "button", "aria-label": "Create event", text: "Create event" });
  create.addEventListener("click", () => history.replaceState(null, "", "/dev/facebook/events/123456789/?show_created_event_toast=true"));

  d.append(file, cover, host, name, dateField("Start date"), timeField("Start time", "11:00"), tzField(), addEnd, end, mode, loc, details, addTicket, ticket, create);
})();
