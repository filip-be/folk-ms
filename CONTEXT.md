# folk-ms — Domain Context

Glossary for the CMS serving bal folk organizations' websites. Terms only; no implementation. Sections follow the capabilities in [docs/README.md](docs/README.md).

## Organizations

**Organization**
An association or group that organizes dance events (e.g. Bal Folk Poznań, Celtycki Wrocław). Owns its Events.

## Event management

**Event**
The CMS's record of one Facebook Event of an Organization, shown on its Site: a bal, jam session, concert, workshop, festival. Belongs to exactly one Organization; is not shared between Organizations (two Organizations co-hosting one Facebook Event each get their own Event). Comes from Sync, or is a Manual Event. Has bilingual content and may have a Cover Image.
_Avoid_: "post" (that is a Social Post).

**Facebook Event**
An event page on Facebook; the source of an Event. Created and edited only on Facebook by a Moderator; the CMS never creates, changes or removes one. A repeating Facebook Event has one event page per date.
_Avoid_: "event" alone (that is an Event), "post" (that is a Social Post).

**Sync**
Pulling an Organization's Facebook Events, those its Facebook page created, into the CMS: new ones become Events, changed ones update theirs, cancelled ones are shown as cancelled, and one Facebook confirms is gone moves to the Bin. Runs when a Moderator presses "Sync now", and hourly while Autosync is on. Never undoes a Moderator's Override, Hide or CMS text, and never touches a Manual Event.

**Autosync**
An Organization's opt-in to hourly Sync. While it is on, the CMS keeps a Facebook page token for the Organization; Meta lets that token read data for 90 days after the sign-in that issued it, so someone signs in with Meta again at least that often. Off by default.

**Occurrence**
One date of an Event, with its own Facebook Event link. A repeating Facebook Event yields one Event with several Occurrences.

**Override**
A CMS value replacing one Facebook-sourced field of a synced Event (name, description, Location, Cover Image, ticket link). It wins over Facebook. When Facebook later changes that field, the Override stays and the Event is flagged until the Moderator keeps it or follows Facebook again. Dates and cancellation cannot be overridden.

**Manual Event**
An Event a Moderator enters by hand for a Facebook Event another Facebook page created, typically one the Organization co-hosts, which Sync cannot read. Has its Occurrences entered one by one and a required link to its Facebook Event. The Moderator keeps it up to date.

**Hidden**
An Event kept off its Site by a Moderator. Sync never shows it again.

**Location**
Where an Occurrence happens, as free text (a place name, an address), with an optional map link.

**Price**
Free-text, bilingual description of the cost of attending, with an optional tickets/registration link.

**Summary**
A short bilingual description of an Event for lists on its Site.

**Cover Image**
The landscape image representing an Event on Sites and in Facebook Social Posts; the Facebook Event's cover unless overridden.

**Social Image**
An optional square/portrait image (e.g. a poster) used for Instagram Social Posts. When absent, a crop of the Cover Image is used.

**Bin**
Where Events go when Facebook confirms their Facebook Event is gone, or when a Moderator removes a Manual Event. The Super Admin can restore Events from the Bin for 30 days, after which the Event and its Social Post records are gone; the Audit Log still remembers them. Nothing in the Bin is removed from Facebook or Instagram.

## Site delivery

**Site**
A public website of one Organization. The set of Sites is fixed and declared together with the CMS. Each Site is bilingual (Polish and English).

**Permalink**
The stable address of an Event on its Site, based on an identifier that never changes (the readable part may change without breaking it). Addresses the whole Event; an Event with several Occurrences lists them all. Social Posts link to it, so Hidden Events and Events in the Bin cannot get Social Posts.

**Translation fallback**
An Event's Polish text comes from Facebook (or the Moderator, for a Manual Event); English is added in the CMS and optional. Where English is missing, Sites show Polish.

## Social posting

**Social Post**
An announcement of an Event on an Organization's social media account: a Facebook page feed post or an Instagram image post. Distinct from the Event itself and from a Facebook Event. Posted manually by a Moderator: while Autosync is off, the Moderator signs in with Meta every time, and one sign-in can post on both platforms; while it is on, the CMS posts with the Facebook page token it keeps, after a recent CMS sign-in. Each platform gets its own Social Post. The CMS remembers each Social Post's outcome (pending, posted, failed). A post whose outcome is unknown is retried only after the Moderator confirms it did not go out.

**Caption**
The text of a Social Post. Generated from the Event's fields and Permalink, then edited by the Moderator before posting.

**Social Account Connection**
Which Facebook page and Instagram account an Organization syncs from and posts to. Belongs to the Organization, not to the person who made it. Established by a Moderator who signs in with Meta and chooses the Facebook page; any of the Organization's Moderators can change or disconnect it. Grants no access by itself while Autosync is off; while it is on, the CMS also keeps a Facebook page token for it. Without it, the Organization cannot Sync or post Social Posts.

**Post Request**
A Moderator's request to post an Event on Facebook, Instagram or both, with its Captions, kept for a few minutes while they sign in with Meta. Used once; nothing is posted until the sign-in completes it. Its sibling, a Sync Request, runs one Sync with the sign-in's token.

## Access

**Super Admin**
The single person who administers the CMS and grants or revokes Moderator access. Can act as Moderator of every Organization. Signs in only with the Google identity it was bound to.

**Moderator**
A person allowed to manage the Events and the Social Account Connection of the Organizations they were granted. Access is scoped per Organization. Signs in with Google or Facebook; any identity that returns the Moderator's email is the same Moderator.

**Recent sign-in**
A sign-in to the CMS with Google or Facebook in the last 15 minutes. Needed, while Autosync is on, to post a Social Post, to turn Autosync on or off, and to change or remove the Social Account Connection.

**Invitation**
A Super Admin's grant of Moderator access to a person, identified by email, for chosen Organizations. Becomes bound to that person's identity on their first sign-in; expires after 14 days if unused and can be revoked before then.

**Audit Log**
An append-only record of who did what and when: Event changes, Syncs, Hide, Social Posts, Autosync and connection changes, access grants. Readable by the Super Admin.

## Future extensions (not in v1)

**Video Collection**
A curated, categorized set of instructional dance videos (links), shown on a Site.
