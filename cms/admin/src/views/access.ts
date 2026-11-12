import { api, type Invitation, type User } from "../api.ts";
import { busy, fmtDate, fmtDateTime, html, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

const INV_STATUS: Record<string, string> = { pending: L("oczekuje", "pending"), bound: L("przyjęte", "accepted"), expired: L("wygasło", "expired"), revoked: L("odwołane", "revoked") };

class AccessView extends View {
  private users: User[] = [];
  private invs: Invitation[] = [];

  async load(): Promise<void> {
    await this.refresh();
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      const f = e.target as HTMLFormElement;
      if (f.id === "invite") void this.invite(f);
      else void this.saveOrgs(f);
    });
    this.on("[data-revoke-inv]", "click", (_e, el) => {
      if (!confirm(L("Odwołać zaproszenie?", "Revoke the invitation?"))) return;
      void this.act(el as HTMLButtonElement, () => api.revokeInvitation(el.dataset.revokeInv!), L("Odwołano zaproszenie", "Invitation revoked"));
    });
    this.on("[data-revoke-user]", "click", (_e, el) => {
      if (!confirm(L("Odebrać cały dostęp? Działa od następnego kliknięcia tej osoby.", "Revoke all access? It takes effect on this person's next click."))) return;
      void this.act(el as HTMLButtonElement, () => api.revokeUser(el.dataset.revokeUser!), L("Odebrano dostęp", "Access revoked"));
    });
  }

  private async refresh(): Promise<void> {
    [this.users, this.invs] = await Promise.all([api.users(), api.invitations()]);
    this.render();
  }

  private orgBoxes(selected: string[]): ReturnType<typeof html> {
    return html`${this.me.orgs.map(
      (o) => html`<label class="chip"><input type="checkbox" name="orgs" value="${o.id}" ${selected.includes(o.id) ? "checked" : ""}> ${t(o.name)}</label>`,
    )}`;
  }

  private render(): void {
    const loginUrl = `${location.origin}/app/`;
    this.set(html`
      <h1>${L("Moderatorzy", "Moderators")}</h1>
      <section class="card">
        <h2>${L("Zaproś moderatora", "Invite a moderator")}</h2>
        <form id="invite">
          <p class="required-note">${L("wymagane", "required")}</p>
          <div class="field"><label for="invite-email">${L("E-mail konta Google lub Facebook", "Google or Facebook account email")}</label><input id="invite-email" name="email" type="email" required></div>
          <fieldset data-field="orgs"><legend class="required">${L("Organizacje", "Organizations")}</legend>${this.orgBoxes([])}</fieldset>
          <button class="primary" type="submit">${L("Zaproś", "Invite")}</button>
        </form>
        <p class="muted small">${L("CMS nie wysyła e-maili. Przekaż osobie link", "The CMS sends no email. Give the person the link")} <code>${loginUrl}</code> – ${L("zaproszenie wiąże się z kontem, którym zaloguje się pierwszy raz, i wygasa po 14 dniach. Potem może logować się przez Google lub Facebooka, o ile konto zwraca ten sam adres e-mail.", "the invitation is bound to the account they first sign in with and expires after 14 days. After that they can sign in with Google or Facebook, as long as the account returns the same email address.")}</p>
      </section>

      <section class="card">
        <h2>${L("Zaproszenia", "Invitations")}</h2>
        ${this.invs.length
          ? html`<div class="table-wrap"><table class="list"><thead><tr><th>${L("E-mail", "Email")}</th><th>${L("Organizacje", "Organizations")}</th><th>${L("Stan", "Status")}</th><th>${L("Wygasa", "Expires")}</th><th></th></tr></thead><tbody>
            ${this.invs.map(
              (i) => html`<tr><td>${i.email}</td><td>${i.orgs.join(", ")}</td><td><span class="badge ${i.effectiveStatus}">${INV_STATUS[i.effectiveStatus] ?? i.effectiveStatus}</span></td>
                <td>${fmtDate(i.expiresAt)}</td>
                <td>${i.effectiveStatus === "pending" ? html`<button type="button" class="link danger" data-revoke-inv="${i.id}">${L("Odwołaj", "Revoke")}</button>` : ""}</td></tr>`,
            )}</tbody></table></div>`
          : html`<p class="muted">${L("Brak zaproszeń.", "No invitations.")}</p>`}
      </section>

      <section class="card">
        <h2>${L("Użytkownicy", "Users")}</h2>
        <ul class="users">${this.users.map(
          (u) => html`<li class="${u.revoked ? "revoked" : ""}">
            <div><strong>${u.email}</strong> ${u.superAdmin ? html`<span class="badge published">${L("administrator", "administrator")}</span>` : ""}
              ${u.revoked ? html`<span class="badge binned">${L("bez dostępu", "no access")}</span>` : ""}
              <div class="muted small">${L("ostatnie logowanie", "last sign-in")} ${fmtDateTime(u.lastLoginAt)}</div></div>
            ${u.superAdmin || u.revoked
              ? ""
              : html`<form data-user="${u.id}" class="inline-form">${this.orgBoxes(u.orgs ?? [])}
                  <button type="submit">${L("Zapisz", "Save")}</button>
                  <button type="button" class="link danger" data-revoke-user="${u.id}">${L("Odbierz dostęp", "Revoke access")}</button></form>`}
          </li>`,
        )}</ul>
      </section>`);
  }

  private async invite(f: HTMLFormElement): Promise<void> {
    const fd = new FormData(f);
    await this.act(f.querySelector("button"), () => api.invite(String(fd.get("email")), fd.getAll("orgs").map(String)), L("Utworzono zaproszenie", "Invitation created"), f);
  }

  private async saveOrgs(f: HTMLFormElement): Promise<void> {
    const orgs = new FormData(f).getAll("orgs").map(String);
    await this.act(f.querySelector("button[type=submit]"), () => api.setUserOrgs(f.dataset.user!, orgs), L("Zapisano", "Saved"), f);
  }

  private async act(btn: HTMLButtonElement | null, fn: () => Promise<unknown>, ok: string, form?: HTMLFormElement): Promise<void> {
    await busy(btn, async () => {
      try {
        await fn();
        toast(ok);
        await this.refresh();
      } catch (err) {
        showError(err, null, form);
      }
    });
  }
}

customElements.define("folk-access", AccessView);
