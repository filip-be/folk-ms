// Lazy access to the vendored abcjs (ADR 0012). The admin and the Sites import
// this module; Bun splits abcjs into its own chunk, which loads on the first
// loadAbcjs() call, so only Tune screens and pages download it.
//
//   const abcjs = await loadAbcjs();
//   const [tune] = abcjs.renderAbc(element, abc, { responsive: "resize", add_classes: true });
//   if (tune && abcjs.synth.supportsAudio()) {
//     const synth = new abcjs.synth.CreateSynth();
//     await synth.init({ visualObj: tune, options: synthOptions("/sounds/", "fiddle") });
//     await synth.prime();
//     synth.start(); // inside a click handler: browsers start audio only on a user gesture
//   }

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
  loading ??= import("./6.7.3/abcjs-basic-min.js").then((m) => m.default as Abcjs);
  return loading;
}

// Minimal declarations for the abcjs 6.7.3 API used here (the full ones are in
// the npm package's types/index.d.ts, which is not vendored).

export type Selector = string | HTMLElement;

export interface AbcVisualParams {
  add_classes?: boolean;
  ariaLabel?: string;
  responsive?: "resize";
  scale?: number;
  staffwidth?: number;
  paddingtop?: number;
  paddingbottom?: number;
  paddingleft?: number;
  paddingright?: number;
  foregroundColor?: string;
  selectionColor?: string;
  oneSvgPerLine?: boolean;
  print?: boolean;
  visualTranspose?: number;
  wrap?: { minSpacing: number; maxSpacing: number; preferredMeasuresPerLine: number; lastLineLimit?: number };
  clickListener?: (abcElem: unknown, tuneNumber: number, classes: string, analysis: unknown, drag: unknown, mouseEvent: MouseEvent) => void;
  /** Warnings of the parse are on the TuneObject; set this to stop drawing on the first one. */
  stop_on_warning?: boolean;
}

export interface TuneObject {
  warnings?: string[];
  getBpm(): number;
  getTotalTime(): number;
  millisecondsPerMeasure(bpm?: number): number;
  getMeterFraction(): { num: number; den: number };
}

export interface SynthOptions {
  soundFontUrl?: string;
  soundFontVolumeMultiplier?: number;
  program?: number;
  chordsOff?: boolean;
  midiTranspose?: number;
  qpm?: number;
  defaultQpm?: number;
  onEnded?: (context: unknown) => void;
}

export interface MidiBufferInit {
  visualObj?: TuneObject;
  audioContext?: AudioContext;
  millisecondsPerMeasure?: number;
  options?: SynthOptions;
}

export interface MidiBuffer {
  init(params?: MidiBufferInit): Promise<unknown>;
  prime(): Promise<{ status: string; duration: number }>;
  start(): void;
  pause(): number;
  resume(): void;
  seek(position: number, units?: "seconds" | "beats" | "percent"): void;
  stop(): number;
  getIsRunning(): boolean;
}

export interface CursorControl {
  onReady?(): void;
  onStart?(): void;
  onFinished?(): void;
  onBeat?(beatNumber: number, totalBeats: number, totalTime: number): void;
  onEvent?(event: { milliseconds: number; elements?: HTMLElement[][] }): void;
}

export interface SynthController {
  load(selector: Selector, cursorControl?: CursorControl | null, visualOptions?: { displayLoop?: boolean; displayRestart?: boolean; displayPlay?: boolean; displayProgress?: boolean; displayWarp?: boolean }): void;
  setTune(visualObj: TuneObject, userAction: boolean, audioParams?: SynthOptions): Promise<{ status: string }>;
  play(): void;
  pause(): void;
  restart(): void;
  toggleLoop(): void;
  setWarp(percent: number): Promise<void>;
  disable(isDisabled: boolean): void;
}

export interface TimingCallbacks {
  start(position?: number, units?: "seconds" | "beats" | "percent"): void;
  pause(): void;
  reset(): void;
  stop(): void;
}

export interface Abcjs {
  signature: string;
  renderAbc(target: Selector, abc: string, params?: AbcVisualParams): TuneObject[];
  synth: {
    supportsAudio(): boolean;
    activeAudioContext(): AudioContext | undefined;
    registerAudioContext(ac?: AudioContext): boolean;
    CreateSynth: new () => MidiBuffer;
    SynthController: new () => SynthController;
  };
  TimingCallbacks: new (
    visualObj: TuneObject,
    options?: { qpm?: number; beatSubdivisions?: number; beatCallback?: (beat: number, totalBeats: number, totalTime: number) => void; eventCallback?: (event: { milliseconds: number } | null) => void },
  ) => TimingCallbacks;
}
