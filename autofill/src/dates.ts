// Date and time text for Facebook's form. Facebook parses typed dates in
// its UI locale and silently keeps the old date when it cannot, so Autofill
// types each candidate format in turn and checks what Facebook kept.

const MONTHS: Record<"en" | "pl", { short: string[]; long: string[] }> = {
  en: {
    short: ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"],
    long: ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"],
  },
  pl: {
    short: ["sty", "lut", "mar", "kwi", "maj", "cze", "lip", "sie", "wrz", "paź", "lis", "gru"],
    long: ["stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca", "lipca", "sierpnia", "września", "października", "listopada", "grudnia"],
  },
};

export interface YMD {
  y: number;
  m: number; // 1..12
  d: number;
}

export function parseYMD(s: string): YMD | undefined {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) return undefined;
  return { y: +m[1]!, m: +m[2]!, d: +m[3]! };
}

const pad = (n: number) => String(n).padStart(2, "0");

/**
 * Formats a date with a pattern of d, dd, M, MM, MMM, MMMM, yyyy. Month
 * names are in lang.
 */
export function formatDate(date: YMD, pattern: string, lang: "en" | "pl" = "en"): string {
  return pattern.replace(/yyyy|MMMM|MMM|MM|M|dd|d/g, (tok) => {
    switch (tok) {
      case "yyyy":
        return String(date.y);
      case "MMMM":
        return MONTHS[lang].long[date.m - 1]!;
      case "MMM":
        return MONTHS[lang].short[date.m - 1]!;
      case "MM":
        return pad(date.m);
      case "M":
        return String(date.m);
      case "dd":
        return pad(date.d);
      default:
        return String(date.d);
    }
  });
}

/** The texts to try, in order: each pattern in English, then in Polish. */
export function dateCandidates(date: YMD, patterns: string[]): string[] {
  const out: string[] = [];
  for (const p of patterns) {
    for (const lang of ["en", "pl"] as const) {
      const s = formatDate(date, p, lang);
      if (!out.includes(s)) out.push(s);
    }
  }
  return out;
}

/**
 * Reports whether a text Facebook shows (an input value or an aria-label
 * such as "Start date, selected: 20 Dec 2026" or "Data rozpoczęcia, wybrano: 10 lis 2026") names this date. It accepts
 * a month by number or by its English or Polish name.
 */
export function showsDate(text: string, date: YMD): boolean {
  const t = text.toLowerCase();
  const sel = /(?:selected|wybrano):\s*(.+)$/i.exec(text);
  const s = (sel ? sel[1]! : text).toLowerCase();
  if (!s.includes(String(date.y))) return false;
  // Numeric: d/m/y, d.m.y, y-m-d (month must not be read as the day).
  const nums = s.match(/\d+/g)?.map(Number) ?? [];
  if (nums.length >= 3) {
    const [a, b, c] = nums as [number, number, number];
    if (a === date.y && b === date.m && c === date.d) return true;
    if (c === date.y && a === date.d && b === date.m && !/[a-ząćęłńóśźż]{3}/.test(s)) return true;
  }
  const names = [...MONTHS.en.short, ...MONTHS.en.long, ...MONTHS.pl.short, ...MONTHS.pl.long].map((n) => n.toLowerCase());
  const want = [MONTHS.en.short, MONTHS.en.long, MONTHS.pl.short, MONTHS.pl.long].map((l) => l[date.m - 1]!.toLowerCase());
  const words = s.match(/[a-ząćęłńóśźż]+/g) ?? [];
  const month = words.find((w) => names.includes(w));
  if (!month || !want.includes(month)) return false;
  return nums.includes(date.d) && t.length > 0;
}

/** Time candidates for "HH:MM" (24-hour first, then 12-hour). */
export function timeCandidates(hhmm: string, patterns: string[]): string[] {
  const m = /^(\d{2}):(\d{2})$/.exec(hhmm);
  if (!m) return [];
  const h = +m[1]!;
  const min = m[2]!;
  const out: string[] = [];
  for (const p of patterns) {
    if (p === "HH:mm") out.push(`${pad(h)}:${min}`);
    else if (p === "h:mm a") out.push(`${h % 12 || 12}:${min} ${h < 12 ? "AM" : "PM"}`);
  }
  return out;
}

/** Reports whether a time Facebook shows equals "HH:MM". */
export function showsTime(text: string, hhmm: string): boolean {
  const m = /(\d{1,2}):(\d{2})\s*([ap]\.?m\.?)?/i.exec(text);
  if (!m) return false;
  let h = +m[1]!;
  const ap = m[3]?.toLowerCase().replace(/\./g, "");
  if (ap === "pm" && h < 12) h += 12;
  if (ap === "am" && h === 12) h = 0;
  return `${pad(h)}:${m[2]}` === hhmm;
}
