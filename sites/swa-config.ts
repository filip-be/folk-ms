// The Static Web App config a build deploys. A Site's staticwebapp.config.json
// may name the public storage origin (where the snapshots, images and Tune
// Attachments are served) as STORAGE_ORIGIN; the origin is known only at
// deploy time, so build.ts replaces the token with the origin of SNAPSHOT_URL
// (vars.STORAGE_PUBLIC_URL in deploy-site.yml, the local CMS otherwise).

export const STORAGE_ORIGIN = "{{STORAGE_ORIGIN}}";

/** The origin (scheme, host and port) of the snapshot address. */
export function storageOrigin(snapshotUrl: string): string {
  return new URL(snapshotUrl).origin;
}

/** The config text with the storage origin filled in. */
export function swaConfig(text: string, snapshotUrl: string): string {
  return text.replaceAll(STORAGE_ORIGIN, storageOrigin(snapshotUrl));
}

/** The sources of one CSP directive ("frame-src" → its sources), or undefined. */
export function cspSources(csp: string, directive: string): string[] | undefined {
  const d = csp
    .split(";")
    .map((x) => x.trim().split(/\s+/))
    .find((x) => x[0] === directive);
  return d?.slice(1);
}
