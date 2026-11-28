// Pieces shared by the Tune Collection screens (tunes.ts, tune.ts) and shown
// on /app/design: bilingual text where either language may come first, Keys,
// the lists' labels, and the ABC sheet with piano playback (abcjs, ADR 0013).

import type { Bi, TuneCollection, TuneKey, TuneName } from "../api.ts";
import { icon } from "../icons.ts";
import { L, lang } from "../i18n.ts";
import { html, type Raw } from "../ui.ts";
import { loadAbcjs, synthOptions, type MidiBuffer, type TuneObject } from "../../../../notation/abcjs.ts";

/** Bilingual text in the UI language, else the other language (either may be written first). */
export const biText = (b?: Bi): string => (lang === "en" ? b?.en || b?.pl : b?.pl || b?.en) || "";

/** Both languages of a list label, as the lists show it: "pl / en". */
export const biBoth = (b: Bi): string => (b.pl && b.en && b.pl !== b.en ? `${b.pl} / ${b.en}` : b.pl || b.en || "");

/** A Tune's name with its translation in the UI language, if any. */
export function nameText(n: TuneName): string {
  const tr = biText(n.translation);
  return tr && tr !== n.name ? `${n.name} (${tr})` : n.name;
}

/** Tonics a Key can have (the CMS reads A–G with ♯ or ♭). */
export const TONICS = ["C", "C#", "Db", "D", "D#", "Eb", "E", "F", "F#", "Gb", "G", "G#", "Ab", "A", "A#", "Bb", "B"];
export const tonicLabel = (t: string) => t.replace(/^([A-G])#$/, "$1♯").replace(/^([A-G])b$/, "$1♭");

const MODE_LABEL: Record<string, string> = {
  major: L("dur", "major"),
  minor: L("moll", "minor"),
  dorian: L("dorycka", "dorian"),
  mixolydian: L("miksolidyjska", "mixolydian"),
  lydian: L("lidyjska", "lydian"),
  phrygian: L("frygijska", "phrygian"),
  locrian: L("lokrycka", "locrian"),
};
export const modeLabel = (m: string) => MODE_LABEL[m] ?? m;
export const keyText = (k?: TuneKey) => (k?.tonic ? `${tonicLabel(k.tonic)} ${modeLabel(k.mode)}` : "");

export const typeText = (c: TuneCollection, id: string) => biText(c.types.find((x) => x.id === id)?.label);
export const instrumentText = (c: TuneCollection, id: string) => biText(c.instruments.find((x) => x.id === id)?.label);

/** An Origin as text: "Region, Country" or "Country". */
export function tuneOriginText(c: TuneCollection, id?: string): string {
  if (!id) return "";
  for (const co of c.origins) {
    if (co.id === id) return biText(co.label);
    const r = co.regions.find((x) => x.id === id);
    if (r) return `${biText(r.label)}, ${biText(co.label)}`;
  }
  return "";
}

/** "Used by N tunes", in the UI language. */
export function tuneUsesText(n: number): string {
  if (!n) return L("nieużywane", "not used");
  const pl = n === 1 ? "melodii" : "melodiach";
  return L(`w ${n} ${pl}`, `in ${n} tune${n === 1 ? "" : "s"}`);
}

/** Text folded for search: lower case, without diacritics (as the CMS's Fold). */
export const fold = (s: string) =>
  s
    .toLowerCase()
    .replace(/ł/g, "l")
    .replace(/ß/g, "ss")
    .normalize("NFD")
    .replace(/\p{M}/gu, "");

/** The note a Playback Tempo counts, from the Time Signature (as domain.TempoBeat). */
export function tempoBeat(meter: string): string {
  const [n, d] = meter.split("/");
  if (!d) return "1/4";
  if (d === "8" && ["3", "6", "9", "12"].includes(n ?? "")) return "3/8";
  if (d === "2") return "1/2";
  return `1/${d}`;
}

/**
 * The ABC with its tempo: the header's Q: is replaced, or one is added
 * before K: (as withTempo in contract/client/src/tunes.ts, which the admin
 * project does not build).
 */
export function withTempo(abc: string, tempo: number, beat: string): string {
  const lines = abc.split(/\r?\n/);
  const q = `Q:${beat}=${Math.round(tempo)}`;
  const k = lines.findIndex((l) => /^K:/.test(l));
  const existing = lines.slice(0, k < 0 ? lines.length : k).findIndex((l) => /^Q:/.test(l));
  if (existing >= 0) lines[existing] = q;
  else if (k >= 0) lines.splice(k, 0, q);
  else lines.splice(lines.findIndex((l) => /^X:/.test(l)) + 1, 0, q);
  return lines.join("\n");
}

/** The Play / Stop button of a sheet. */
export function playButton(id: string, disabled = true): Raw {
  return html`<button type="button" class="play" id="${id}" aria-pressed="false" ${disabled ? "disabled" : ""}>${icon("play")}<span>${L("Odtwórz", "Play")}</span></button>`;
}

/** The audio of the sheet playing now: one at a time on a screen. */
let playing: Sheet | undefined;

/**
 * An ABC sheet drawn by abcjs into `el`, with optional piano playback
 * through `button` (playButton). The admin plays piano only, from /app/sounds/.
 */
export class Sheet {
  private abc = "";
  private tempo?: { bpm: number; beat: string };
  private synth?: MidiBuffer;
  /** Warnings of the last drawing, as plain text. */
  warnings: string[] = [];

  constructor(
    private readonly el: HTMLElement,
    private readonly button?: HTMLButtonElement | null,
  ) {
    button?.addEventListener("click", () => void this.toggle());
  }

  /** Draws `abc` (empty clears); `tempo` is what playback follows. */
  async render(abc: string, tempo?: { bpm: number; beat: string }): Promise<boolean> {
    this.stop();
    this.abc = abc.trim();
    this.tempo = tempo;
    if (!this.abc) {
      this.el.replaceChildren();
      this.warnings = [];
      this.setButton(false);
      return false;
    }
    const abcjs = await loadAbcjs();
    const [tune] = abcjs.renderAbc(this.el, this.abc, { responsive: "resize", add_classes: true });
    // abcjs words its warnings as HTML; keep the text only (DOMParser runs no scripts).
    this.warnings = (tune?.warnings ?? []).map((w) => new DOMParser().parseFromString(w, "text/html").body.textContent ?? "");
    const ok = !!tune && this.el.querySelector("svg") !== null;
    this.setButton(ok && abcjs.synth.supportsAudio());
    return ok;
  }

  private setButton(enabled: boolean, on = false): void {
    const b = this.button;
    if (!b) return;
    b.disabled = !enabled;
    b.setAttribute("aria-pressed", String(on));
    b.innerHTML = (on ? html`${icon("stop")}<span>${L("Zatrzymaj", "Stop")}</span>` : html`${icon("play")}<span>${L("Odtwórz", "Play")}</span>`).value;
  }

  /** Plays the sheet, or stops it when it plays. Call from a click: browsers start audio only on a user gesture. */
  async toggle(): Promise<void> {
    if (this.synth) {
      this.stop();
      return;
    }
    playing?.stop();
    playing = this;
    const abcjs = await loadAbcjs();
    abcjs.synth.registerAudioContext();
    void abcjs.synth.activeAudioContext()?.resume();
    const text = this.tempo ? withTempo(this.abc, this.tempo.bpm, this.tempo.beat) : this.abc;
    // "*" parses without drawing: the drawn sheet keeps the Moderator's own Q: (or none).
    const [tune] = abcjs.renderAbc("*", text) as (TuneObject | undefined)[];
    if (!tune) return;
    const synth = new abcjs.synth.CreateSynth();
    this.synth = synth;
    this.setButton(true, true);
    try {
      await synth.init({ visualObj: tune, options: synthOptions("/app/sounds/", "piano", { onEnded: () => this.stop() }) });
      await synth.prime();
      if (this.synth === synth) synth.start();
    } catch (err) {
      console.error(err);
      this.stop();
    }
  }

  stop(): void {
    this.synth?.stop();
    this.synth = undefined;
    if (playing === this) playing = undefined;
    this.setButton(!!this.abc && !!this.button && this.el.querySelector("svg") !== null);
  }
}

/** Stops whatever sheet plays (when leaving a screen). */
export const stopPlayback = () => playing?.stop();

/** A file size as "123 KB" or "4.5 MB". */
export function sizeText(n: number): string {
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}
