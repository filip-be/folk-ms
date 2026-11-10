// Autofill entry, shared by both builds (see build.ts). It runs on the CMS
// origin and on Facebook; when another Autofill already runs in the page
// (extension and userscript both installed), the second one stays idle.

import { runOnCms } from "./cms.ts";
import { extensionKV, scriptHandler, userscriptKV } from "./env.ts";
import { runOnFacebook } from "./facebook.ts";
import { MARKER } from "./protocol.ts";
import { Store } from "./store.ts";

declare const __AF_CHANNEL__: "extension" | "userscript";
declare const __AF_VERSION__: string;
/** The CMS origin; the userscript gets it filled in when the CMS serves it. */
declare const __AF_CMS_ORIGIN__: string;
/** Dev builds also treat the CMS's practice page as Facebook. */
declare const __AF_PRACTICE__: boolean;

const PRACTICE_PATH = "/dev/facebook/";

function browserName(): string {
  const ua = navigator.userAgent;
  const m = /(Firefox|Edg|Chrome|Safari)\/([\d.]+)/.exec(ua);
  const mobile = /Android|iPhone|iPad|Mobile/.test(ua) ? " mobile" : "";
  return (m ? `${m[1]} ${m[2]!.split(".")[0]}` : "unknown") + mobile;
}

function start(): void {
  const html = document.documentElement;
  if (html.hasAttribute(MARKER)) return; // another Autofill runs here
  html.setAttribute(MARKER, `${__AF_CHANNEL__}@${__AF_VERSION__}`);

  const store = new Store(__AF_CHANNEL__ === "extension" ? extensionKV() : userscriptKV());
  const onPractice = __AF_PRACTICE__ && location.origin === __AF_CMS_ORIGIN__ && location.pathname.startsWith(PRACTICE_PATH);
  const host = location.hostname;
  if (onPractice || host === "www.facebook.com" || host === "m.facebook.com" || host === "facebook.com") {
    const handler = scriptHandler();
    void runOnFacebook(store, {
      version: __AF_VERSION__,
      channel: handler ? `${__AF_CHANNEL__} (${handler})` : __AF_CHANNEL__,
      browser: browserName(),
    });
    return;
  }
  runOnCms(store, __AF_CMS_ORIGIN__, { version: __AF_VERSION__, channel: __AF_CHANNEL__ });
}

start();
