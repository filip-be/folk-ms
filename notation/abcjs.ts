// Lazy access to abcjs, the npm package (ADR 0012). The admin and the Sites import
// this module; it import()s the package's prebuilt dist/abcjs-basic-min.js, so Bun
// splits abcjs into its own chunk, which loads on the first loadAbcjs() call: only
// Tune screens and pages download it. The types are the package's own
// (types/index.d.ts); abcjs-dist.d.ts maps the dist file to them.
//
//   const abcjs = await loadAbcjs();
//   const [tune] = abcjs.renderAbc(element, abc, { responsive: "resize", add_classes: true });
//   if (tune && abcjs.synth.supportsAudio()) {
//     const synth = new abcjs.synth.CreateSynth();
//     await synth.init({ visualObj: tune, options: synthOptions("/sounds/", "fiddle") });
//     await synth.prime();
//     synth.start(); // inside a click handler: browsers start audio only on a user gesture
//   }

import type { SynthOptions } from "abcjs";

export type { MidiBuffer, SynthOptions, TuneObject } from "abcjs";

/** The abcjs module, as loadAbcjs() resolves it. */
export type Abcjs = typeof import("abcjs");

/** The instruments with self-hosted samples: abcjs sample folder and General MIDI program. */
export const INSTRUMENTS = {
  piano: { sample: "acoustic_grand_piano", program: 0 },
  flute: { sample: "flute", program: 73 },
  fiddle: { sample: "violin", program: 40 },
  accordion: { sample: "accordion", program: 21 },
} as const;
export type Instrument = keyof typeof INSTRUMENTS;

/** The CC BY 3.0 credit to show wherever the samples play. */
export const SOUNDFONT_CREDIT = "Instrument sounds: FluidR3 GM by Frank Wen, CC BY 3.0";

/**
 * Synth options for the self-hosted FluidR3 GM samples. soundFontUrl is where the
 * `<sample>-mp3/` folders are served: "/app/sounds/" in the admin (piano only),
 * "/sounds/" on a Site. Chords are off: the samples cover the melody range only.
 */
export function synthOptions(soundFontUrl: string, instrument: Instrument, extra: SynthOptions = {}): SynthOptions {
  return { soundFontUrl, soundFontVolumeMultiplier: 3.0, chordsOff: true, program: INSTRUMENTS[instrument].program, ...extra };
}

let loading: Promise<Abcjs> | undefined;

/** Loads abcjs once (a separate chunk); later calls return the same module. */
export function loadAbcjs(): Promise<Abcjs> {
  loading ??= import("abcjs/dist/abcjs-basic-min.js").then((m) => m.default);
  return loading;
}
