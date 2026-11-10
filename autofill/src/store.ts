// Autofill's local storage: one pending handoff (single use, 15 minutes),
// undelivered results and queued failure reports. Backed by the extension's
// storage.local or the userscript manager's GM values, behind one
// interface.

import { HANDOFF_TTL_MS, RESULT_TTL_MS, type FillResult, type Handoff, type Report } from "./protocol.ts";

export interface KV {
  get<T>(key: string): Promise<T | undefined>;
  set(key: string, value: unknown): Promise<void>;
  del(key: string): Promise<void>;
}

/** In-memory KV (tests). */
export class MemoryKV implements KV {
  readonly data = new Map<string, string>();
  async get<T>(key: string): Promise<T | undefined> {
    const v = this.data.get(key);
    return v === undefined ? undefined : (JSON.parse(v) as T);
  }
  async set(key: string, value: unknown): Promise<void> {
    this.data.set(key, JSON.stringify(value));
  }
  async del(key: string): Promise<void> {
    this.data.delete(key);
  }
}

const K = { pending: "pending", results: "results", reports: "reports" };
const MAX_QUEUE = 50;

interface Pending {
  handoff: Handoff;
  expiresAt: number;
}

export class Store {
  constructor(
    private readonly kv: KV,
    private readonly now: () => number = Date.now,
  ) {}

  /** Keeps one pending handoff, replacing any earlier one. */
  async putHandoff(h: Handoff): Promise<void> {
    await this.kv.set(K.pending, { handoff: h, expiresAt: this.now() + HANDOFF_TTL_MS } satisfies Pending);
  }

  /** Reports whether a fresh handoff waits (without taking it). */
  async hasHandoff(): Promise<boolean> {
    const p = await this.kv.get<Pending>(K.pending);
    return !!p && p.expiresAt > this.now();
  }

  /** Takes the pending handoff: it can be used once. */
  async takeHandoff(): Promise<Handoff | undefined> {
    const p = await this.kv.get<Pending>(K.pending);
    if (!p) return undefined;
    await this.kv.del(K.pending);
    return p.expiresAt > this.now() ? p.handoff : undefined;
  }

  async addResult(r: FillResult): Promise<void> {
    const list = (await this.results()).filter((x) => x.id !== r.id);
    list.push(r);
    await this.kv.set(K.results, list.slice(-MAX_QUEUE));
  }

  /** Undelivered results younger than a day. */
  async results(): Promise<FillResult[]> {
    const list = (await this.kv.get<FillResult[]>(K.results)) ?? [];
    return list.filter((r) => this.now() - Date.parse(r.at) < RESULT_TTL_MS);
  }

  async addReport(r: Report): Promise<void> {
    const list = (await this.kv.get<Report[]>(K.reports)) ?? [];
    list.push(r);
    await this.kv.set(K.reports, list.slice(-MAX_QUEUE));
  }

  async reports(): Promise<Report[]> {
    return (await this.kv.get<Report[]>(K.reports)) ?? [];
  }

  /** Drops delivered results and reports. */
  async ack(ids: string[]): Promise<void> {
    const keep = <T extends { id: string }>(l: T[]) => l.filter((x) => !ids.includes(x.id));
    await this.kv.set(K.results, keep(await this.results()));
    await this.kv.set(K.reports, keep(await this.reports()));
  }
}
