import type { Me } from "./api.ts";
import { showError, type Raw } from "./ui.ts";

/** Base class of routed screens (light DOM, styled by app.css). */
export abstract class View extends HTMLElement {
  params: Record<string, string> = {};
  me!: Me;

  get org(): string {
    return this.params.org ?? "";
  }

  connectedCallback(): void {
    this.innerHTML = `<p class="muted loading">Ładowanie…</p>`;
    this.load().catch((err) => {
      this.innerHTML = `<div class="card"><p>Nie udało się wczytać danych.</p></div>`;
      showError(err);
    });
  }

  abstract load(): Promise<void>;

  set(content: Raw): void {
    this.innerHTML = content.value;
  }

  $<T extends Element = HTMLElement>(sel: string): T {
    const el = this.querySelector<T>(sel);
    if (!el) throw new Error(`missing ${sel}`);
    return el;
  }

  on(sel: string, type: string, fn: (e: Event, el: HTMLElement) => void): void {
    this.addEventListener(type, (e) => {
      const el = (e.target as HTMLElement).closest<HTMLElement>(sel);
      if (el && this.contains(el)) fn(e, el);
    });
  }
}
