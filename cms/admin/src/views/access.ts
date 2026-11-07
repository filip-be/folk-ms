import { api, type Invitation, type User } from "../api.ts";
import { busy, fmtDate, fmtDateTime, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";

const INV_STATUS: Record<string, string> = { pending: "oczekuje", bound: "przyjęte", expired: "wygasło", revoked: "odwołane" };

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
      if (!confirm("Odwołać zaproszenie?")) return;
      void this.act(el as HTMLButtonElement, () => api.revokeInvitation(el.dataset.revokeInv!), "Odwołano zaproszenie");
    });
    this.on("[data-revoke-user]", "click", (_e, el) => {
      if (!confirm("Odebrać cały dostęp? Działa od następnego kliknięcia tej osoby.")) return;
      void this.act(el as HTMLButtonElement, () => api.revokeUser(el.dataset.revokeUser!), "Odebrano dostęp");
    });
  }

  private async refresh(): Promise<void> {
    [this.users, this.invs] = await Promise.all([api.users(), api.invitations()]);
    this.render();
  }

  private orgBoxes(selected: string[]): ReturnType<typeof html> {
    return html`${this.me.orgs.map(
      (o) => html`<label class="chip"><input type="checkbox" name="orgs" value="${o.id}" ${selected.includes(o.id) ? "checked" : ""}> ${o.name.pl}</label>`,
    )}`;
  }

  private render(): void {
    const loginUrl = `${location.origin}/app/`;
    this.set(html`
      <h1>Moderatorzy</h1>
      <section class="card">
        <h2>Zaproś moderatora</h2>
        <form id="invite">
          <div class="field"><label>E-mail konta Google<input name="email" type="email" required></label></div>
          <fieldset><legend>Organizacje</legend>${this.orgBoxes([])}</fieldset>
          <button class="primary" type="submit">Zaproś</button>
        </form>
        <p class="muted small">CMS nie wysyła e-maili. Przekaż osobie link <code>${loginUrl}</code> – zaproszenie wiąże się z jej kontem Google przy pierwszym logowaniu i wygasa po 14 dniach.</p>
      </section>

      <section class="card">
        <h2>Zaproszenia</h2>
        ${this.invs.length
          ? html`<table class="list"><thead><tr><th>E-mail</th><th>Organizacje</th><th>Stan</th><th>Wygasa</th><th></th></tr></thead><tbody>
            ${this.invs.map(
              (i) => html`<tr><td>${i.email}</td><td>${i.orgs.join(", ")}</td><td><span class="badge ${i.effectiveStatus}">${INV_STATUS[i.effectiveStatus] ?? i.effectiveStatus}</span></td>
                <td>${fmtDate(i.expiresAt)}</td>
                <td>${i.effectiveStatus === "pending" ? html`<button type="button" class="link danger" data-revoke-inv="${i.id}">Odwołaj</button>` : ""}</td></tr>`,
            )}</tbody></table>`
          : html`<p class="muted">Brak zaproszeń.</p>`}
      </section>

      <section class="card">
        <h2>Użytkownicy</h2>
        <ul class="users">${this.users.map(
          (u) => html`<li class="${u.revoked ? "revoked" : ""}">
            <div><strong>${u.email}</strong> ${u.superAdmin ? html`<span class="badge published">administrator</span>` : ""}
              ${u.revoked ? html`<span class="badge binned">bez dostępu</span>` : ""}
              <div class="muted small">ostatnie logowanie ${fmtDateTime(u.lastLoginAt)}</div></div>
            ${u.superAdmin || u.revoked
              ? ""
              : html`<form data-user="${u.id}" class="inline-form">${this.orgBoxes(u.orgs ?? [])}
                  <button type="submit">Zapisz</button>
                  <button type="button" class="link danger" data-revoke-user="${u.id}">Odbierz dostęp</button></form>`}
          </li>`,
        )}</ul>
        <p class="muted small">Odebranie organizacji osobie, która połączyła jej konto Facebook, pokaże moderatorom ostrzeżenie o ponownym połączeniu.</p>
      </section>`);
  }

  private async invite(f: HTMLFormElement): Promise<void> {
    const fd = new FormData(f);
    await this.act(f.querySelector("button"), () => api.invite(String(fd.get("email")), fd.getAll("orgs").map(String)), "Utworzono zaproszenie");
  }

  private async saveOrgs(f: HTMLFormElement): Promise<void> {
    const orgs = new FormData(f).getAll("orgs").map(String);
    await this.act(f.querySelector("button[type=submit]"), () => api.setUserOrgs(f.dataset.user!, orgs), "Zapisano");
  }

  private async act(btn: HTMLButtonElement | null, fn: () => Promise<unknown>, ok: string): Promise<void> {
    await busy(btn, async () => {
      try {
        await fn();
        toast(ok);
        await this.refresh();
      } catch (err) {
        showError(err);
      }
    });
  }
}

customElements.define("folk-access", AccessView);
