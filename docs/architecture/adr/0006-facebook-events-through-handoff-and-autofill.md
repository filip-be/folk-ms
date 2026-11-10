# 0006. Facebook Events through a Handoff and Autofill, not an API

- Status: accepted
- Date: 2026-11-10

## Context

Organizations announce most dances as Facebook Events hosted by their Page. Moderators retyped every Event into Facebook by hand: name, dates, Venue, description, Cover Image, ticket link. The CMS already holds all of it.

The CMS cannot create a Facebook Event for them:

- **API.** The Graph API has not created events since v2.0. The only write path is Meta's Official Events API, which is open to approved ticketing partners only; its [program page](https://developers.facebook.com/products/official-events-api/) says onboarding of new partners is paused. `facebook.com/events/create` has no documented prefill parameters.
- **iframe.** `https://www.facebook.com/events/create/` answers `X-Frame-Options: DENY`, and the same-origin policy would stop the CMS scripting the form anyway.

## Decision

The CMS prepares a **Facebook Event Handoff** and the Moderator creates the Facebook Event on Facebook, acting as the Organization's Page:

- The Handoff screen shows each field with a copy button, the Cover Image to download and "Open Facebook". It works on every device and is the baseline.
- **Autofill** is an optional helper that types the Handoff into Facebook's own create-event form inside the Moderator's own Facebook session. It never presses Create. It brings the new Facebook Event's address back to the CMS, where the Moderator confirms it.
- One Autofill codebase, two builds: a Manifest V3 WebExtension for desktop Chrome/Edge and Firefox, and a userscript the CMS serves at `/app/autofill.user.js` (the instant-fix channel on desktop, the only one on phones: Firefox for Android with Violentmonkey; iOS Safari with a userscript manager on a best-effort basis).
- Facebook's form is found through a field map (`autofill/fieldmap.json`) of ARIA roles and Polish and English labels, never class names. The map is embedded in the admin build and travels with every handoff, so most form changes are fixed by a CMS deploy, not a store release.
- The CMS keeps each Facebook Event's link against its Occurrence, shows it on the Site, and flags it as possibly out of date when the Event changes. Moderators edit Facebook Events by hand; Autofill does not fill the edit form.

## Spike results (2026-11-10)

Checked by hand with a test Page, acting as the Page:

- **Desktop www.facebook.com, English UI:** the create form is a `role=dialog`; every field is a `<label>` whose first line of text names it. Text typed with `execCommand("insertText")` is accepted. Dates and times commit on blur, which needs the tab to have focus. A date in a format the UI locale cannot parse is silently reverted, so Autofill types each candidate format and checks what Facebook kept. The location typeahead keeps free text through its "Just use …" option. The cover photo accepts a `File` set on its `input[type=file]` through `DataTransfer`. The end date and the ticket link sit behind "Add end date and time" and "Add tickets".
- **Event name:** the form counts to 100 characters, not the 64 assumed when this was planned; the Handoff caps the name at 100.
- **After Create**, Facebook navigates client-side (`history.replaceState`) to `/events/{numeric id}/`, so Autofill polls the address.
- **m.facebook.com in a phone viewport:** a simpler form; only the name, description and cover are located by `aria-label`. The other fields are left for the Moderator with copy buttons.
- **Desktop www.facebook.com, Polish UI:** the same form; several labels are not the obvious translations ("Czas rozpoczęcia" but "Godzina zakończenia", "Czy wydarzenie jest offline czy online?" with the option "Offline", "Adres URL strony z biletami"), so the field map lists them as seen. Dates typed in English ("20 Dec 2026") are accepted and shown in Polish ("wybrano: 20 gru 2026"). Inputs have no `name` attributes and React-generated ids, so labels are the only language-independent-enough handle. All fields but the cover (not tried) filled correctly.
- **Not yet checked on the device:** desktop Firefox, Firefox for Android with Violentmonkey, and iOS userscript managers (whether facebook.com jumps into the Facebook app, and whether the manager injects into the CMS page under its CSP). They are the first items of the per-release checklist in the root README; a platform where Autofill fails stays Handoff only.

## Alternatives

- **Proxy Facebook through the CMS** so the CMS origin can script the form: the CMS would handle Moderators' Facebook logins and cookies.
- **Server-side headless browser:** needs stored Facebook sessions, against [ADR 0005](0005-meta-sign-in-for-every-post.md); cannot run on the Flex custom handler; meets bot detection.
- **A ticketing partner such as Eventbrite:** a third party with its own branding, and still a manual click.
- **Handoff only, no Autofill:** no maintenance and no terms risk, but the retyping it was meant to remove stays. Kept as the fallback on every device.

## Consequences

- Moderators still press Create on Facebook, acting as the Page. The CMS never holds a Facebook session or token for this.
- Autofill acts inside Moderators' own Facebook sessions, which Meta's terms restrict. The risk is documented in the root README runbook for Moderators; there is no in-product notice.
- Autofill is expected to break when Facebook changes its form. Breakage degrades to a partial fill: a panel lists every field's status with copy buttons, and the failure is reported to the Audit Log. Most fixes are a field-map change and a CMS deploy; the rest are a store release (the userscript updates from the CMS).
- Each release is tested by hand against real Facebook with the runbook checklist; CI covers the engine against a dev-only practice page and unit tests only.
- A Recurring Event gets one Facebook Event at a time, for its next Occurrence, with the further dates in its description.
