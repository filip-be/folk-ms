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
Pulling an Organization's Facebook Events, those its Facebook page created, into the CMS: new ones become Events, changed ones update theirs, cancelled ones are shown as cancelled, and one Facebook confirms is gone moves to the Bin. Runs when a Moderator presses "Sync now", once when Autosync is turned on, and hourly while it is on. Never undoes a Moderator's Override, Hide or CMS text, and never touches a Manual Event.

**Autosync**
An Organization's opt-in to hourly Sync. Turning it on also runs one Sync right away; Autosync stays on if that Sync fails. While it is on, the CMS keeps a Facebook page token for the Organization; Meta lets that token read data for 90 days after the sign-in that issued it, so someone signs in with Meta again at least that often. Off by default.

**Occurrence**
One date of an Event, with its own Facebook Event link. A repeating Facebook Event yields one Event with several Occurrences.

**Override**
A CMS value replacing one Facebook-sourced field of a synced Event (name, description, Location, Cover Image, ticket link). It wins over Facebook. When Facebook later changes that field, the Override stays and the Event is flagged until the Moderator keeps it or follows Facebook again. Dates and cancellation cannot be overridden.

**Manual Event**
An Event a Moderator enters by hand for a Facebook Event another Facebook page created, typically one the Organization co-hosts, which Sync cannot read. Has its Occurrences entered one by one and a required link to its Facebook Event. The Moderator keeps it up to date.

**Hidden**
An Event or Video kept off its Site by a Moderator. Sync never shows a Hidden Event again.

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
Where Events go when Facebook confirms their Facebook Event is gone, or when a Moderator removes a Manual Event or an Event from a former Facebook page. The Super Admin can restore Events from the Bin for 30 days, after which the Event and its Social Post records are gone; the Audit Log still remembers them. Nothing in the Bin is removed from Facebook or Instagram.

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
A person allowed to manage the Events and the Social Account Connection of the Organizations they were granted. Access is scoped per Organization. Signs in with Google, or with Facebook while its Feature Flag is on; any identity that returns the Moderator's email is the same Moderator.

**Recent sign-in**
A sign-in to the CMS with Google or Facebook in the last 15 minutes. Needed, while Autosync is on, to post a Social Post, to turn Autosync on or off, and to change or remove the Social Account Connection.

**Invitation**
A Super Admin's grant of Moderator access to a person, identified by email, for chosen Organizations. Becomes bound to that person's identity on their first sign-in; expires after 14 days if unused and can be revoked before then.

**Audit Log**
An append-only record of who did what and when: Event changes, Syncs, Hide, Social Posts, Autosync and connection changes, access grants, Feature Flag changes. Readable by the Super Admin.

**Feature Flag**
A switch with which the Super Admin turns a part of the CMS on or off for everyone, without a new release. Only a fixed, known set exists; each has a default. Facebook sign-in is one, off by default.
_Avoid_: "flag" alone (an Event's flags are something else), "setting".

## Video Collection

**Video Collection**
An Organization's curated set of instructional dance Videos, shown on its Site. Every Organization has one, possibly empty. Its Moderators arrange the order of its Videos, and keep its lists of Forms and Origins.

**Video**
One instructional dance video in a Video Collection: a bilingual title and optional description, an optional credit (who dances, teaches or published it), an optional Origin, any number of Forms, and a link to the video on YouTube or Vimeo with an optional start time. The same video may appear in several Videos (e.g. different start times of one long tutorial). Can be Hidden. Removing a Video deletes it; the Audit Log remembers it.
_Avoid_: "item", "clip"; "Event" or "post" (those are other things).

**Form**
How a dance is danced (solo, pair, chain, circle, …), from the Organization's open, bilingual list. A Form may have an optional bilingual description, which a Site may show. A Video has any number of Forms. A Form cannot be removed while a Video has it.
_Avoid_: "type", "category".

**Origin**
Where a dance or a Tune comes from: a country, or a region of a country, from a bilingual list the Video Collection and the Tune Collection each keep for themselves. A Video or a Tune has at most one Origin; a country includes its regions. An Origin cannot be removed while a Video or Tune (or, for a country, one of its regions) uses it.
_Avoid_: "region" alone (a region is one kind of Origin).

## Tune Collection

**Tune Collection**
An Organization's catalogue of the tunes played at its sessions, shown on its Site. Every Organization has one, possibly empty. Belongs to exactly one Organization; its Moderators keep it.
_Avoid_: "repertoire", "tunebook" (until defined).

**Tune**
One piece of music in a Tune Collection, shown on its Site so musicians can find it, hear it and share it. Has a name and any number of Other Names, a Tune Type, a Key and a Time Signature, and may have a Transcription, Instruments it Suits or is Tricky on, and Attachments.
_Avoid_: "song" (tunes are instrumental), "Video" (that is something else).

**Other Name**
Another name a Tune is known by (an Irish-language title, a regional name). A Tune's name and each Other Name are kept as musicians know them, with an optional Polish and English translation; all of them find the Tune.
_Avoid_: "alias", "alternative title".

**Transcription**
A Tune's written notation in ABC, at most one per Tune, from which its sheet music and playback come. A Tune may have none yet. When entered, it proposes the Tune's Key and Time Signature; the Moderator decides them.
_Avoid_: "setting" (thesession.org's word for one of several versions; a Tune here has one), "notes" (ambiguous).

**Key**
The key a Tune is played in, as a tonic and a mode (G major, E minor, A dorian, D mixolydian). A Tune that changes key between parts has one Key, the one it starts in; the change shows on its sheet.

**Time Signature**
The metre a Tune is written in (4/4, 6/8, 9/8, …). Each Tune Type proposes one (reel 4/4, jig 6/8); the Moderator may choose another for a Tune.
_Avoid_: "rhythm" (ambiguous with Tune Type).

**Playback Tempo**
How fast the Site plays a Tune's Transcription or Incipit: the Tune Type's default unless the Moderator sets one for the Tune, and the visitor may slow it down or speed it up. Not how fast the session plays it; the musicians decide that.

**Instrument**
An instrument musicians bring to sessions, from the Organization's bilingual list. A Tune may list Instruments it **Suits** (sits well on it) and Instruments it is **Tricky on** (harder to play there, never forbidden).
_Avoid_: "discouraged", "not recommended".

**Composer**
Who wrote a Tune, as free text; "trad." when the Tune is traditional or its composer unknown. Optional.

**Recording**
A link to someone playing a Tune: a video on YouTube or Vimeo, which plays on the Site (optionally from a start time), or a page elsewhere (SoundCloud, Bandcamp, …), which the Site links to. A Tune may have several, in the order the Moderator sets, each with an optional label (who plays it).
_Avoid_: "Video" (that is an instructional dance video in the Video Collection).

**Incipit**
The opening bars of a Tune's Transcription, shown and played in the catalogue so musicians recognise a Tune they cannot name: by default the pickup and the first two bars; the Moderator may choose one to eight. A Tune without a Transcription has none.

**Attachment**
An image or PDF a Moderator adds to a Tune for more detail (a scan of an old sheet, a photo, a typeset score), shown on the Tune's page in the order the Moderator sets, with an optional bilingual caption.

**Tune Type**
The dance rhythm a Tune is played in, as Irish musicians name it: reel, jig, slip jig, hornpipe, polka, slide, waltz, …, from the Tune Collection's bilingual list, which starts with the common Irish types. A Tune has exactly one.
_Avoid_: "Form" (that is how a dance is danced), "rhythm" (ambiguous with the time signature).

**Set**
A few Tunes played back to back at a session (say three reels), in order, made by a Moderator, with an optional name (otherwise its Tunes' names). A Tune may be in several Sets. Shown on its Site with its own page.
_Avoid_: "medley", "list"; "selection" (that is what a visitor ticks to print).
