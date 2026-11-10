// The two builds' storage: the WebExtension's storage.local, or the
// userscript manager's promise-based GM.getValue/setValue/deleteValue (with
// a shim for managers that only offer GM_*). No value-change listeners.

import type { KV } from "./store.ts";

interface ExtStorage {
  get(key: string): Promise<Record<string, unknown>>;
  set(items: Record<string, unknown>): Promise<void>;
  remove(key: string): Promise<void>;
}
interface GMApi {
  getValue(key: string, def?: unknown): Promise<unknown> | unknown;
  setValue(key: string, value: unknown): Promise<void> | void;
  deleteValue(key: string): Promise<void> | void;
  info?: { scriptHandler?: string };
}

declare const browser: { storage: { local: ExtStorage } } | undefined;
declare const chrome: { storage: { local: ExtStorage } } | undefined;
declare const GM: GMApi | undefined;
declare const GM_getValue: ((key: string, def?: unknown) => unknown) | undefined;
declare const GM_setValue: ((key: string, value: unknown) => void) | undefined;
declare const GM_deleteValue: ((key: string) => void) | undefined;
declare const GM_info: { scriptHandler?: string } | undefined;

const PREFIX = "folkms.";

export function extensionKV(): KV {
  const s = (typeof browser !== "undefined" && browser?.storage ? browser : chrome)!.storage.local;
  return {
    async get<T>(key: string) {
      const v = (await s.get(PREFIX + key))[PREFIX + key];
      return typeof v === "string" ? (JSON.parse(v) as T) : undefined;
    },
    set: (key, value) => s.set({ [PREFIX + key]: JSON.stringify(value) }),
    del: (key) => s.remove(PREFIX + key),
  };
}

function gmApi(): GMApi {
  if (typeof GM !== "undefined" && GM && typeof GM.getValue === "function") return GM;
  if (typeof GM_getValue === "function" && typeof GM_setValue === "function" && typeof GM_deleteValue === "function") {
    return { getValue: GM_getValue, setValue: GM_setValue, deleteValue: GM_deleteValue };
  }
  throw new Error("no GM storage: the userscript manager must grant GM.getValue/GM.setValue/GM.deleteValue");
}

export function userscriptKV(): KV {
  const gm = gmApi();
  return {
    async get<T>(key: string) {
      const v = await gm.getValue(PREFIX + key);
      return typeof v === "string" ? (JSON.parse(v) as T) : undefined;
    },
    async set(key, value) {
      await gm.setValue(PREFIX + key, JSON.stringify(value));
    },
    async del(key) {
      await gm.deleteValue(PREFIX + key);
    },
  };
}

/** The userscript manager's name, for reports. */
export function scriptHandler(): string {
  try {
    if (typeof GM !== "undefined" && GM?.info?.scriptHandler) return GM.info.scriptHandler;
    if (typeof GM_info !== "undefined" && GM_info?.scriptHandler) return GM_info.scriptHandler;
  } catch {
    /* not a userscript */
  }
  return "";
}
