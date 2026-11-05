# folk-ms — Domain Context

Glossary for the CMS serving bal folk organizations' websites. Terms only; no implementation.

## Terms

**Organization**
An association or group that organizes dance events (e.g. Bal Folk Poznań, Celtycki Wrocław). Owns its Events.

**Site**
A public website of one Organization. The set of Sites is fixed and declared together with the CMS. Each Site is bilingual (Polish and English).

**Event**
Something an Organization organizes and announces: a bal, jam session, concert, workshop, festival. Belongs to exactly one Organization; is not shared between Organizations. Has bilingual content and may have a Cover Image. May be single, multi-day, or recurring.
_Avoid_: "post" (that is a Social Post).

**Recurring Event**
An Event defined once with a Recurrence, which yields Occurrences.

**Recurrence**
The repetition rule of a Recurring Event, with a start date and a mandatory end date. One of: weekly on a weekday; every second week on a weekday; monthly on the Nth (or last) weekday of the month.

**Occurrence**
One concrete dated instance of an Event. A Recurring Event's single Occurrence can be cancelled or changed without touching the others.

**Multi-day Event**
An Event whose single Occurrence spans more than one day.

**Duplicate**
Creating a new Draft Event from an existing (typically past) Event, copying its content but not its dates or Social Posts. The two Events are independent afterwards. The usual way to announce the next jam or bal when no Recurrence is defined.

**Venue**
A place where Events happen: name, address, optional map link. Kept as a reusable list per Organization. Editing a Venue immediately changes every not-yet-happened Occurrence that uses it; Occurrences that already happened keep the Venue as it was. A Venue is never deleted, only archived (hidden from choice, still shown where used).

**Price**
Free-text, bilingual description of the cost of attending, with an optional tickets/registration link.

**Cover Image**
The landscape image representing an Event on Sites and in Facebook Social Posts.

**Social Image**
An optional square/portrait image (e.g. a poster) used for Instagram Social Posts. When absent, a crop of the Cover Image is used.

**Permalink**
The stable address of an Event on its Site, based on an identifier that never changes (the readable part may change without breaking it). Addresses the whole Event; a Recurring Event's Permalink lists its Occurrences. Social Posts link to it, so only Published Events can get Social Posts.

**Draft**
The editable state of an Event. Changes to a Draft are invisible to Sites.

**Publish**
A Moderator's explicit action that makes an Event's current Draft the version shown on its Organization's Site. Re-publishing replaces the shown version; Unpublish removes the Event from the Site.

**Translation fallback**
An Event is authored in Polish; English is optional. Where English is missing, Sites show Polish.

**Super Admin**
The single person who administers the CMS and grants or revokes Moderator access. Can act as Moderator of every Organization.

**Moderator**
A person allowed to manage the Events of the Organizations they were granted. Access is scoped per Organization.

**Invitation**
A Super Admin's grant of Moderator access to a person, identified by email, for chosen Organizations. Becomes bound to that person's identity on their first sign-in; expires after 14 days if unused and can be revoked before then.

**Unpublish**
Removing a Published Event from its Site. The Event stays in the CMS as a Draft.

**Bin**
Where deleted Events go. Deleting a Published Event also Unpublishes it. The Super Admin can restore Events from the Bin for 30 days, after which the Event and its Social Post records are gone; the Audit Log still remembers them. Deleting never removes anything from Facebook or Instagram.

**Social Post**
An announcement of a Published Event on an Organization's social media account: a Facebook Page feed post or an Instagram image post. Distinct from the Event itself and from a Facebook Event (which is created by hand outside the CMS). Posted manually by a Moderator, separately per platform; the CMS remembers each Social Post's outcome (pending, posted, failed). A post whose outcome is unknown is retried only after the Moderator confirms it did not go out.

**Caption**
The text of a Social Post. Generated from the Event's fields and Permalink, then edited by the Moderator before posting.

**Social Account Connection**
The link between an Organization and its Facebook Page and Instagram account. Belongs to the Organization, not to the person who made it. Established by a Moderator who administers that Page and was given a role on the CMS's Meta app by the Super Admin. Any of the Organization's Moderators can disconnect or reconnect it. Without it, the Organization cannot post Social Posts.

**Audit Log**
An append-only record of who did what and when: Event changes, Publish/Unpublish, Social Posts, access grants. Readable by the Super Admin.

## Future extensions (not in v1)

**Video Collection**
A curated, categorized set of instructional dance videos (links), shown on a Site.
