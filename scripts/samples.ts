// The instrument samples the Tune Collection plays (ADR 0012): FluidR3 GM per-note
// MP3s in the layout abcjs loads, `<soundFontUrl><instrument>-mp3/<note>.mp3`.
// Shared by fetch-samples.ts and its test.
import { join } from "node:path";

export const SOUNDFONT_DIR = join(import.meta.dir, "..", "vendor", "soundfonts", "FluidR3_GM");
// gleitz/midi-js-soundfonts at a fixed commit of its gh-pages branch.
export const SOURCE = "https://raw.githubusercontent.com/gleitz/midi-js-soundfonts/044fab8e1456bfafc5776e86dfd6bb8697149aef/FluidR3_GM";
// abcjs instrument names (General MIDI program 0, 73, 40, 21).
export const INSTRUMENTS = ["acoustic_grand_piano", "flute", "violin", "accordion"] as const;
// MIDI notes G3 (55) to D7 (98): the range of the tunes, melody only (chordsOff).
export const FIRST_NOTE = 55;
export const LAST_NOTE = 98;

const NAMES = ["C", "Db", "D", "Eb", "E", "F", "Gb", "G", "Ab", "A", "Bb", "B"];

/** The note name abcjs asks for (src/synth/pitch-to-note-name.js): 61 → "Db4". */
export function noteName(midi: number): string {
  return `${NAMES[midi % 12]}${Math.floor(midi / 12) - 1}`;
}

/** Every sample path relative to SOUNDFONT_DIR, e.g. "flute-mp3/A4.mp3". */
export function samplePaths(): string[] {
  const paths: string[] = [];
  for (const i of INSTRUMENTS) for (let n = FIRST_NOTE; n <= LAST_NOTE; n++) paths.push(`${i}-mp3/${noteName(n)}.mp3`);
  return paths;
}
