const EASE = "cubic-bezier(0.2, 0, 0, 1)";
const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const featuredQueries = [
  "Breaking Bad",
  "Chernobyl",
  "Stranger Things",
  "The Bear",
  "Sherlock",
  "Game of Thrones",
  "The Sopranos",
  "The Office",
];

async function api(path, options) {
  const init = options || {};
  const headers = {};
  let body;
  if (init.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(init.body);
  }
  let response;
  try {
    response = await fetch(path, {
      method: init.method || "GET",
      headers,
      body,
      credentials: "same-origin",
    });
  } catch {
    throw new Error("Could not reach the program.");
  }
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error((data.error && data.error.message) || "Something went wrong. Try again.");
    error.status = response.status;
    error.code = data.error && data.error.code;
    throw error;
  }
  return data;
}

function gone(err) {
  if (err.status === 401) {
    location.replace("login.html");
    return true;
  }
  return false;
}

function mapCard(card) {
  return {
    id: card.id,
    titleId: card.titleId || card.id,
    name: card.name,
    year: card.year,
    episodes: card.episodeCount,
    average: card.averageMinutes,
    kind: card.kind || "",
    poster: card.poster || "",
    assembling: Boolean(card.assembling),
    about: card.summary || "",
    genres: card.genres || [],
  };
}

function mapItem(item) {
  const card = mapCard(item);
  card.id = item.id;
  card.titleId = item.titleId;
  card.seasonId = item.seasonId || 0;
  card.seasonNumber = item.seasonNumber || 0;
  card.totalEpisodes = item.seasonEpisodes || item.episodeCount;
  card.remainingEpisodes = item.remainingEpisodes;
  card.remainingMinutes = item.remainingMinutes || 0;
  if (item.remainingEpisodes != null) card.episodes = item.remainingEpisodes;
  return card;
}

function posterNode(item, extra) {
  const letter = item.name ? item.name.slice(0, 1) : "";
  const fallback = () => {
    const span = document.createElement("span");
    span.className = "poster fallback" + (extra ? " " + extra : "");
    span.textContent = letter;
    return span;
  };
  if (!item.poster) return fallback();
  const img = document.createElement("img");
  img.className = "poster" + (extra ? " " + extra : "");
  img.alt = "";
  img.src = item.poster;
  img.addEventListener("error", () => img.replaceWith(fallback()));
  return img;
}

function known(item) {
  return item.episodes > 0 && item.average > 0;
}

function kindOf(item) {
  return item.kind === "film" ? "film" : "series";
}

function plural(n, one, other) {
  return n === 1 ? one : other;
}

function formatMinutes(total) {
  if (total < 60) return total + " min";
  const hours = Math.floor(total / 60);
  const minutes = total % 60;
  return minutes ? hours + " hr " + minutes + " min" : hours + " hr";
}

function finishOffset(remaining, daily) {
  if (remaining <= 0 || daily <= 0) return null;
  return Math.ceil(remaining / daily) - 1;
}

function finishDay(offset) {
  const date = new Date();
  date.setHours(12, 0, 0, 0);
  date.setDate(date.getDate() + offset);
  let small = date.toLocaleDateString("en-US", { weekday: "long" });
  if (offset === 0) return { big: "Today", small };
  if (offset === 1) return { big: "Tomorrow", small };
  if (date.getFullYear() !== new Date().getFullYear()) small += ", " + date.getFullYear();
  return { big: date.toLocaleDateString("en-US", { month: "short", day: "numeric" }), small };
}

function rhythmParts(rows, daily) {
  const current = rows.find((item) => (item.remainingMinutes || 0) > 0);
  const queueMinutes = rows.reduce((sum, item) => sum + Math.max(0, item.remainingMinutes || 0), 0);
  const unknown = rows.some((item) => (item.remainingMinutes || 0) <= 0 && (item.episodes || 0) > 0);
  if (!current) {
    if (rows.length && rows.every((item) => (item.episodes || 0) === 0)) return { text: "Everything in the queue is already watched." };
    return { text: "Nothing in the queue has a runtime to count yet." };
  }
  if (daily <= 0) return { text: "How many minutes a day do you watch? I’ll show when the current season ends and when the whole queue ends." };
  return {
    tiles: [
      Object.assign({ label: "Season ends", name: current.name }, finishDay(finishOffset(current.remainingMinutes, daily))),
      Object.assign({ label: "Queue ends", name: "at " + daily + " min a day" }, finishDay(finishOffset(queueMinutes, daily)))
    ],
    text: unknown ? "Titles without a runtime are left out." : ""
  };
}

function readCount(value) {
  const text = value.trim();
  if (!/^\d+$/.test(text)) return null;
  const number = Number(text);
  if (!Number.isInteger(number)) return null;
  return number;
}

function readDaily(form) {
  const raw = form.daily.value.trim();
  if (raw === "") return 0;
  const minutes = Number(raw);
  if (!Number.isInteger(minutes) || minutes < 0 || minutes > 1440) return null;
  return minutes;
}

function seriesWord(n) {
  return plural(n, "episode", "episodes");
}

function runtimeLine(item) {
  if (item.kind === "film") {
    if (!item.average) return "runtime unknown";
    return formatMinutes(item.average);
  }
  return item.episodes + " " + seriesWord(item.episodes) + ", " + item.average + " min on average";
}

function mountDetail(root, item, queued) {
  root.innerHTML = `
    <div class="detail">
      <div class="detail-poster"></div>
      <div class="detail-copy">
        <p class="facts"></p>
        <p class="blurb"></p>
        ${item.kind === "series"
          ? '<div class="detail-actions"><a class="primary" href="title.html?id=' + item.id + '">Seasons</a>'
            + (item.assembling ? "" : '<button class="ghost" type="button" data-whole="' + item.id + '">Whole series</button>')
            + "</div>"
          : (queued ? '<span class="queued">In the queue</span>' : '<button class="primary" type="button" data-add="">Add to queue</button>')}
      </div>
    </div>`;
  root.querySelector(".detail-poster").append(posterNode(item, "poster-lg"));
  root.querySelector(".facts").textContent = item.year + " · " + runtimeLine(item);
  const blurb = root.querySelector(".blurb");
  if (item.about) blurb.textContent = item.about;
  else blurb.remove();
  const button = root.querySelector("[data-add]");
  if (button) button.dataset.add = item.titleId;
}

function outcomeText(line) {
  if (line.outcome === "full") return "in full · " + formatMinutes(line.minutes);
  if (line.outcome === "partial") return line.episodes + " " + seriesWord(line.episodes) + " · " + formatMinutes(line.minutes);
  if (line.outcome === "stopped") return "does not fit";
  return "runtime unknown";
}

function readFree(form) {
  const hours = Number(form.hours.value);
  const minutes = Number(form.minutes.value);
  if (!Number.isInteger(hours) || !Number.isInteger(minutes) || hours < 0 || minutes < 0) return 0;
  const total = hours * 60 + minutes;
  if (total <= 0 || total > 7 * 24 * 60) return 0;
  return total;
}

function readTime(form) {
  const hours = Number(form.hours.value);
  const minutes = Number(form.minutes.value);
  if (!Number.isInteger(hours) || !Number.isInteger(minutes)) return null;
  return { hours, minutes };
}

function showError(text) {
  const alert = document.querySelector(".alert");
  if (!alert) return;
  alert.querySelector("p").textContent = text;
  alert.classList.add("show");
}

function clearError() {
  const alert = document.querySelector(".alert");
  if (!alert) return;
  alert.classList.remove("show");
}

function setBusy(button, on) {
  document.querySelector(".bar").classList.toggle("on", on);
  if (!button) return;
  button.classList.toggle("is-loading", on);
  button.disabled = on;
}

function validName(value) {
  const chars = Array.from(value);
  if (chars.length < 3 || chars.length > 32) return false;
  return chars.every((ch) => ch === "_" || /\p{L}|\p{N}/u.test(ch));
}

function paintChrome(user) {
  const page = document.body.dataset.page;
  document.querySelectorAll(".nav a").forEach((link) => {
    if (link.dataset.page === page) link.setAttribute("aria-current", "page");
  });
  const who = document.querySelector(".who");
  const logout = document.querySelector("#logout");
  if (!who || !user) return;
  who.textContent = user.username;
  logout.hidden = false;
  logout.addEventListener("click", async () => {
    try {
      await api("/api/logout", { method: "POST" });
    } catch {
      /* cookie is cleared on the next login either way */
    }
    location.href = "login.html";
  });
}

function bindAuth(mode) {
  const form = document.querySelector("form");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    const username = form.username.value.trim();
    const password = form.password.value;
    if (!validName(username)) {
      showError("Name: 3 to 32 characters, letters, digits, and underscore only.");
      return;
    }
    if (Array.from(password).length < 8) {
      showError("Password: at least 8 characters.");
      return;
    }
    const email = form.email ? form.email.value.trim() : "";
    if (mode === "register" && (!email.includes("@") || email.length < 6)) {
      showError("Enter an email address.");
      return;
    }
    const button = form.querySelector("button");
    setBusy(button, true);
    try {
      if (mode === "register") {
        await api("/api/register", { method: "POST", body: { username, password, email } });
      }
      await api("/api/login", { method: "POST", body: { username, password } });
      location.href = "queue.html";
    } catch (err) {
      setBusy(button, false);
      showError(err.message);
    }
  });
}

function bindAccountMail(user) {
  const form = document.querySelector(".account-mail");
  if (!form) return;
  const note = document.querySelector(".account-mail-note");
  form.email.value = user.email || "";
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    note.hidden = true;
    const email = form.email.value.trim();
    if (email !== "" && (!email.includes("@") || email.length < 6)) {
      showError("Enter an email address.");
      return;
    }
    const button = form.querySelector("button");
    setBusy(button, true);
    try {
      const data = await api("/api/me", { method: "PATCH", body: { email } });
      form.email.value = data.email || "";
      note.textContent = data.email ? "Address saved. The Thursday letter will come here." : "Address removed.";
      note.hidden = false;
    } catch (err) {
      showError(err.message);
    } finally {
      setBusy(button, false);
    }
  });
}

function bindForgot() {
  const form = document.querySelector("form");
  const notice = document.querySelector(".notice");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    notice.hidden = true;
    const email = form.email.value.trim();
    if (!email.includes("@") || email.length < 6) {
      showError("Enter an email address.");
      return;
    }
    const button = form.querySelector("button");
    setBusy(button, true);
    try {
      const data = await api("/api/password/forgot", { method: "POST", body: { email } });
      notice.textContent = data.message || "If that address exists, the letter has been sent.";
      notice.hidden = false;
    } catch (err) {
      showError(err.message);
    } finally {
      setBusy(button, false);
    }
  });
}

function bindReset() {
  const form = document.querySelector("form");
  const token = new URLSearchParams(location.search).get("token") || "";
  if (!token) showError("This link has expired. Request a new one.");
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    const password = form.password.value;
    if (Array.from(password).length < 8) {
      showError("Password: at least 8 characters.");
      return;
    }
    const button = form.querySelector("button");
    setBusy(button, true);
    try {
      await api("/api/password/reset", { method: "POST", body: { token, password } });
      location.href = "login.html";
    } catch (err) {
      setBusy(button, false);
      showError(err.message);
    }
  });
}

function bindSearch() {
  const form = document.querySelector(".search-form");
  const hint = document.querySelector(".hint");
  const hits = document.querySelector(".hits");
  const manualForm = document.querySelector(".manual");
  const manualLead = document.querySelector(".manual-lead");
  const manualOpen = document.querySelector(".manual-open");
  let openId = "";
  let timer = 0;
  let seq = 0;
  let queued = new Set();

  api("/api/queue").then((data) => {
    queued = new Set((data.items || []).map((item) => item.titleId));
    hits.querySelectorAll("[data-add]").forEach((button) => {
      if (queued.has(Number(button.dataset.add))) markQueued(button);
    });
  }).catch((err) => {
    if (!gone(err)) showError(err.message);
  });

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    clearTimeout(timer);
    runSearch(true);
  });

  form.q.addEventListener("input", () => {
    clearTimeout(timer);
    timer = setTimeout(() => runSearch(false), 220);
  });

  hits.addEventListener("click", async (event) => {
    const whole = event.target.closest("[data-whole]");
    if (whole) {
      try {
        await api("/api/queue", { method: "POST", body: { titleId: Number(whole.dataset.whole), whole: true } });
        markQueued(whole);
      } catch (err) {
        if (gone(err)) return;
        if (err.code === "ALREADY_IN_QUEUE") markQueued(whole);
        showError(err.message);
      }
      return;
    }
    const add = event.target.closest("[data-add]");
    if (add) {
      const titleId = Number(add.dataset.add);
      try {
        await api("/api/queue", { method: "POST", body: { titleId } });
        queued.add(titleId);
        markQueued(add);
      } catch (err) {
        if (gone(err)) return;
        if (err.code === "ALREADY_IN_QUEUE") {
          queued.add(titleId);
          markQueued(add);
        }
        showError(err.message);
      }
      return;
    }
    const main = event.target.closest(".hit-main");
    if (!main) return;
    const hit = main.closest(".hit");
    const id = hit.dataset.id;
    hits.querySelectorAll(".hit").forEach((node) => {
      if (node !== hit) {
        node.classList.remove("open");
        node.querySelector(".fold").inert = true;
      }
    });
    const willOpen = openId !== id;
    hit.classList.toggle("open", willOpen);
    hit.querySelector(".fold").inert = !willOpen;
    openId = willOpen ? id : "";
  });

  function hideManual() {
    manualForm.hidden = true;
    manualOpen.hidden = true;
  }

  function showManual(lead) {
    manualLead.textContent = lead;
    manualForm.hidden = false;
    manualOpen.hidden = true;
  }

  function offerManual() {
    if (!manualForm.hidden) {
      manualOpen.hidden = true;
      return;
    }
    manualOpen.hidden = false;
  }

  manualOpen.addEventListener("click", () => {
    showManual("Year, episode count, and minutes per episode. A film is one episode.");
  });

  manualForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    const name = form.q.value.trim();
    const year = readCount(manualForm.year.value);
    const episodes = readCount(manualForm.episodes.value);
    const average = readCount(manualForm.average.value);
    if (Array.from(name).length < 2 || Array.from(name).length > 120) {
      showError("Title: 2 to 120 characters.");
      return;
    }
    if (year == null || year < 1888 || year > 2100) {
      showError("Year: from 1888 to 2100.");
      return;
    }
    if (episodes == null || episodes < 1 || episodes > 500) {
      showError("Episodes: from 1 to 500.");
      return;
    }
    if (average == null || average < 1 || average > 1440) {
      showError("Episode length: from 1 to 1440 minutes.");
      return;
    }
    const button = manualForm.querySelector("button");
    setBusy(button, true);
    try {
      const card = await api("/api/titles", {
        method: "POST",
        body: { name, year, episodeCount: episodes, averageMinutes: average },
      });
      const item = mapCard(card);
      hits.prepend(renderHit(item, queued.has(item.titleId), 0));
      manualForm.reset();
      manualForm.hidden = true;
      manualOpen.hidden = false;
    } catch (err) {
      if (!gone(err)) showError(err.message);
    } finally {
      setBusy(button, false);
    }
  });

  function clearFilters() {
    const box = document.querySelector(".filters");
    box.replaceChildren();
    box.hidden = true;
  }

  function paintFilters(cards) {
    const box = document.querySelector(".filters");
    const genres = [];
    const seen = new Set();
    cards.forEach((card) => {
      (card.genres || []).forEach((genre) => {
        if (!genre || seen.has(genre)) return;
        seen.add(genre);
        genres.push(genre);
      });
    });
    box.replaceChildren();
    if (genres.length < 2) {
      box.hidden = true;
      return;
    }
    box.hidden = false;
    const all = document.createElement("button");
    all.type = "button";
    all.className = "filter";
    all.textContent = "All";
    all.setAttribute("aria-pressed", "true");
    all.addEventListener("click", () => applyGenre(""));
    box.append(all);
    genres.forEach((genre) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "filter";
      button.textContent = genre;
      button.addEventListener("click", () => applyGenre(genre));
      box.append(button);
    });
  }

  function applyGenre(genre) {
    const box = document.querySelector(".filters");
    box.querySelectorAll(".filter").forEach((button) => {
      button.setAttribute("aria-pressed", String(button.textContent === (genre || "All")));
    });
    hits.querySelectorAll(".hit").forEach((hit) => {
      const list = (hit.dataset.genres || "").split(",").filter(Boolean);
      hit.hidden = genre !== "" && !list.includes(genre);
    });
  }

  function markQueued(button) {
    const mark = document.createElement("span");
    mark.className = "queued";
    mark.textContent = "In the queue";
    button.replaceWith(mark);
  }

  async function runSearch(fromSubmit) {
    const query = form.q.value.trim();
    const token = ++seq;
    if (Array.from(query).length < 2) {
      hits.replaceChildren();
      clearFilters();
      openId = "";
      hideManual();
      if (fromSubmit) {
        hint.hidden = true;
        showError("Enter at least two characters.");
      } else {
        clearError();
        hint.hidden = false;
        hint.textContent = "Enter a title";
      }
      return;
    }
    clearError();
    hint.hidden = true;
    hideManual();
    clearFilters();
    showSkeletons();
    const button = form.querySelector("button");
    if (fromSubmit) setBusy(button, true);
    try {
      const data = await api("/api/titles?q=" + encodeURIComponent(query));
      if (token !== seq) return;
      hits.replaceChildren();
      openId = "";
      const titles = data.titles || [];
      titles.forEach((card, index) => {
        const item = mapCard(card);
        hits.append(renderHit(item, queued.has(item.titleId), index));
      });
      paintFilters(titles);
      if ((data.titles || []).length) offerManual();
      else showManual("Not in the catalog. Year, episode count, and minutes per episode. A film is one episode.");
    } catch (err) {
      if (token !== seq) return;
      hits.replaceChildren();
      clearFilters();
      if (gone(err)) return;
      if (err.code === "TITLE_NOT_FOUND") {
        clearError();
        showManual("Not in the catalog. Year, episode count, and minutes per episode. A film is one episode.");
        return;
      }
      showError(err.message);
      if (err.code === "CATALOG_UNAVAILABLE") {
        showManual("The catalog did not answer. You can add your own card: year, episode count, and minutes per episode.");
      }
    } finally {
      if (token === seq && fromSubmit) setBusy(button, false);
    }
  }

  function showSkeletons() {
    hits.replaceChildren();
    for (let i = 0; i < 4; i += 1) {
      const row = document.createElement("div");
      row.className = "hit skeleton";
      row.innerHTML = '<span class="sk poster"></span><span class="sk-copy"><span class="sk sk-title"></span><span class="sk sk-meta"></span></span>';
      hits.append(row);
    }
  }

  function renderHit(item, isQueued, index) {
    const hit = document.createElement("article");
    hit.className = "hit";
    hit.dataset.id = item.id;
    hit.dataset.genres = (item.genres || []).join(",");
    hit.style.animationDelay = Math.min(index, 8) * 30 + "ms";
    hit.innerHTML = `
      <button class="hit-main" type="button">
        <span class="hit-copy">
          <span class="hit-name"></span>
          <span class="hit-meta"></span>
        </span>
      </button>
      <div class="fold"><div class="fold-inner"></div></div>`;
    hit.querySelector(".fold").inert = true;
    hit.querySelector(".hit-main").prepend(posterNode(item));
    hit.querySelector(".hit-name").textContent = item.name;
    hit.querySelector(".hit-meta").textContent = item.assembling
      ? (item.year ? item.year + " · " : "") + "assembling"
      : item.year + " · " + kindOf(item);
    mountDetail(hit.querySelector(".fold-inner"), item, isQueued);
    return hit;
  }
}

function bindQueue() {
  const page = document.querySelector(".page");
  const list = document.querySelector(".queue");
  const plan = document.querySelector(".plan");
  const form = document.querySelector(".time");
  const rhythmForm = document.querySelector(".rhythm");
  const rhythmLine = document.querySelector(".rhythm-line");
  const heroDates = document.querySelector(".hero-dates");
  const heroTotal = document.querySelector(".hero-total");
  const letterLine = document.querySelector(".letter");
  const picks = document.querySelector(".picks");
  const pickList = document.querySelector(".pick-list");
  const summary = document.querySelector(".summary");
  const shelf = document.querySelector(".shelf");
  const startOpen = document.querySelector(".start-open");
  const shelfError = document.querySelector(".shelf-error");
  let items = [];
  let featured = null;
  let featuredLoad = null;
  let drag = null;
  let openShelf = "";
  let undoTimer = 0;
  let undone = null;
  let busy = false;
  let planTimer = 0;
  let paceTimer = 0;
  let paceToken = 0;
  let dailyMinutes = 0;
  let letterText = "";
  let suggestions = [];

  function queueMinutes(rows) {
    return rows.reduce((sum, item) => sum + (item.remainingMinutes || (known(item) ? item.episodes * item.average : 0)), 0);
  }

  function summaryText(rows) {
    const minutes = queueMinutes(rows);
    const unknown = rows.filter((item) => !known(item)).length;
    let text = rows.length + " " + plural(rows.length, "title", "titles");
    if (unknown > 0) text += " · " + unknown + " with no runtime";
    const free = readFree(form);
    if (free > 0 && minutes > 0) {
      const ratio = minutes / free;
      if (ratio < 0.5) text += " · shorter than one evening like this";
      else {
        const evenings = Math.max(1, Math.round(ratio));
        text += evenings === 1
          ? " · about one evening like this"
          : " · about " + evenings + " evenings like this";
      }
    }
    return text;
  }

  function showRhythm(parts) {
    if (typeof parts === "string") parts = { text: parts };
    const tiles = (parts && parts.tiles) || [];
    rhythmLine.textContent = (parts && parts.text) || "";
    heroDates.replaceChildren();
    tiles.forEach((tile) => {
      const box = document.createElement("div");
      box.className = "date-tile";
      [["date-label", tile.label], ["date-big", tile.big], ["date-small", tile.small], ["date-name", tile.name]].forEach(([cls, text]) => {
        const line = document.createElement("span");
        line.className = cls;
        line.textContent = text;
        box.append(line);
      });
      heroDates.append(box);
    });
    heroDates.hidden = tiles.length === 0;
  }

  function showTotal() {
    const minutes = queueMinutes(items);
    heroTotal.textContent = "";
    if (minutes <= 0) {
      heroTotal.textContent = "No runtime yet";
    } else {
      formatMinutes(minutes).split(" ").forEach((part) => {
        const span = document.createElement("span");
        span.className = /^\d/.test(part) ? "total-num" : "total-unit";
        span.textContent = part;
        heroTotal.append(span);
      });
    }
    summary.textContent = items.length ? summaryText(items) : "";
  }

  function render(skipFlipId) {
    page.classList.toggle("is-empty", items.length === 0);
    showTotal();
    showRhythm(items.length ? rhythmParts(items, dailyMinutes) : "");
    if (letterLine) {
      letterLine.hidden = letterText === "";
      letterLine.textContent = letterText ? "Letter: " + letterText : "";
    }
    if (picks && pickList) {
      pickList.replaceChildren();
      suggestions.forEach((card) => {
        const link = document.createElement("a");
        link.className = "pick";
        link.href = "title.html?id=" + card.id;
        link.append(posterNode(mapCard(card), "poster-shelf"));
        const name = document.createElement("span");
        name.className = "pick-name";
        name.textContent = card.name + (card.year ? " · " + card.year : "");
        link.append(name);
        if (card.reason) {
          const why = document.createElement("span");
          why.className = "pick-why";
          why.textContent = card.reason;
          link.append(why);
        }
        pickList.append(link);
      });
      picks.hidden = suggestions.length === 0;
    }
    list.replaceChildren();
    items.forEach((item, index) => {
      const row = document.createElement("li");
      row.className = "q-item";
      if (!known(item)) row.classList.add("unknown");
      if (String(item.id) === String(skipFlipId)) row.classList.add("lifting");
      row.dataset.id = item.id;
      const total = item.remainingMinutes || item.episodes * item.average;
      const length = total > 0 ? formatMinutes(total) : (item.totalEpisodes > 0 && item.episodes === 0 ? "all episodes watched" : "runtime unknown");
      const kind = item.kind === "series" && item.seasonNumber ? "season " + item.seasonNumber : kindOf(item);
      const left = item.kind === "series" && item.totalEpisodes ? " · " + item.episodes + " of " + item.totalEpisodes + " left" : "";
      row.innerHTML = `
        <button class="grip" type="button" aria-label="Drag"><span></span></button>
        <a class="q-open">
          <div class="hit-name"></div>
          <div class="hit-meta"></div>
        </a>
        <div class="q-actions">
          <button class="ghost" type="button" data-dir="up">higher</button>
          <button class="ghost" type="button" data-dir="down">lower</button>
          <button class="ghost danger" type="button" data-dir="remove">remove</button>
        </div>`;
      const href = "title.html?id=" + item.titleId + (item.seasonNumber ? "&season=" + item.seasonNumber : "");
      const posterLink = document.createElement("a");
      posterLink.className = "q-poster";
      posterLink.href = href;
      posterLink.tabIndex = -1;
      posterLink.append(posterNode(item));
      row.querySelector(".grip").after(posterLink);
      row.querySelector(".q-open").href = href;
      row.querySelector(".hit-name").textContent = item.name;
      row.querySelector(".hit-meta").textContent = (index + 1) + " · " + kind + " · " + length + left;
      row.querySelector('[data-dir="up"]').disabled = index === 0;
      row.querySelector('[data-dir="down"]').disabled = index === items.length - 1;
      list.append(row);
    });
    if (items.length === 0) {
      openShelf = "";
      if (featured) paintShelf();
      else loadFeatured();
    }
  }

  function flip(first) {
    if (reduce) return;
    [...list.children].forEach((node) => {
      const before = first.get(node.dataset.id);
      if (!before) return;
      const after = node.getBoundingClientRect();
      const dy = before.top - after.top;
      if (!dy) return;
      node.animate(
        [{ transform: "translateY(" + dy + "px)" }, { transform: "translateY(0)" }],
        { duration: 220, easing: EASE }
      );
    });
  }

  function paintShelf() {
    shelf.replaceChildren();
    shelfError.hidden = true;
    featured.forEach((item) => {
      const card = document.createElement("button");
      card.type = "button";
      card.className = "shelf-card";
      card.dataset.shelf = item.titleId;
      card.append(posterNode(item, "poster-shelf"));
      const name = document.createElement("span");
      name.className = "shelf-name";
      name.textContent = item.name;
      card.append(name);
      shelf.append(card);
    });
    startOpen.replaceChildren();
    startOpen.hidden = true;
  }

  function showShelfSkeletons() {
    shelf.replaceChildren();
    shelfError.hidden = true;
    for (let i = 0; i < featuredQueries.length; i += 1) {
      const card = document.createElement("div");
      card.className = "shelf-card";
      const bone = document.createElement("span");
      bone.className = "sk poster poster-shelf";
      card.append(bone);
      shelf.append(card);
    }
  }

  function loadFeatured() {
    if (featuredLoad) return featuredLoad;
    showShelfSkeletons();
    featuredLoad = fetchFeatured().then((cards) => {
      featured = cards;
      if (items.length === 0) paintShelf();
    }).catch((err) => {
      featured = [];
      shelf.replaceChildren();
      if (gone(err)) return;
      shelfError.hidden = false;
      shelfError.textContent = err.message;
    });
    return featuredLoad;
  }

  function paintShelfDetail() {
    shelf.querySelectorAll(".shelf-card").forEach((card) => {
      const on = card.dataset.shelf === openShelf;
      card.classList.toggle("is-open", on);
      card.setAttribute("aria-expanded", on ? "true" : "false");
    });
    const item = openShelf && featured ? featured.find((card) => String(card.titleId) === openShelf) : null;
    startOpen.replaceChildren();
    if (!item) {
      startOpen.hidden = true;
      return;
    }
    startOpen.hidden = false;
    const queued = items.some((row) => String(row.titleId) === String(item.titleId));
    mountDetail(startOpen, item, queued);
  }

  async function reload() {
    const data = await api("/api/queue");
    items = (data.items || []).map(mapItem);
    letterText = data.letter || "";
    suggestions = data.suggestions || [];
    if (document.activeElement !== rhythmForm.daily) {
      dailyMinutes = data.rhythm ? data.rhythm.dailyMinutes || 0 : 0;
      rhythmForm.daily.value = dailyMinutes > 0 ? String(dailyMinutes) : "";
    }
    render();
  }

  async function savePace(value) {
    const token = ++paceToken;
    try {
      const saved = await api("/api/rhythm", { method: "PATCH", body: { dailyMinutes: value } });
      if (token !== paceToken) return;
      dailyMinutes = saved.dailyMinutes || 0;
      await reload();
    } catch (err) {
      if (token !== paceToken || gone(err)) return;
      showError(err.message);
    }
  }

  async function shift(id, direction, steps) {
    let latest = null;
    for (let i = 0; i < steps; i += 1) {
      latest = await api("/api/queue/" + id, { method: "PATCH", body: { direction } });
    }
    if (latest) items = (latest.items || []).map(mapItem);
  }

  function hideUndo() {
    undone = null;
    clearTimeout(undoTimer);
    const toast = document.querySelector(".undo");
    toast.classList.remove("show");
    toast.inert = true;
  }

  function showUndo(item, index) {
    undone = { titleId: item.titleId, seasonId: item.seasonId, index, name: item.name };
    const toast = document.querySelector(".undo");
    toast.querySelector("p").textContent = "Removed “" + item.name + "”";
    toast.inert = false;
    toast.classList.add("show");
    clearTimeout(undoTimer);
    undoTimer = setTimeout(hideUndo, 4000);
  }

  function remove(id, row) {
    const index = items.findIndex((item) => String(item.id) === String(id));
    const item = items[index];
    const finish = async () => {
      busy = true;
      try {
        await api("/api/queue/" + id, { method: "DELETE" });
        items = items.filter((entry) => String(entry.id) !== String(id));
        render();
        if (item) showUndo(item, index);
      } catch (err) {
        if (!gone(err)) showError(err.message);
        try { await reload(); } catch { /* the alert already explains it */ }
      } finally {
        busy = false;
      }
      if (plan.classList.contains("open") && items.every((row) => String(row.id) !== String(id))) {
        fillPlan(false);
      }
    };
    if (reduce) {
      finish();
      return;
    }
    const height = row.offsetHeight;
    row.style.overflow = "hidden";
    const animation = row.animate(
      [
        { height: height + "px", opacity: 1, marginBottom: "8px" },
        { height: "0px", opacity: 0, marginBottom: "0px" },
      ],
      { duration: 200, easing: EASE }
    );
    animation.onfinish = finish;
  }

  function paintBar(result, freeMinutes) {
    const track = plan.querySelector(".timebar-track");
    const bar = plan.querySelector(".timebar");
    track.replaceChildren();
    const bits = [];
    result.lines.forEach((line) => {
      if (line.minutes <= 0) return;
      const seg = document.createElement("div");
      seg.className = "seg " + line.outcome;
      seg.style.flexGrow = String(line.minutes);
      seg.title = line.title + " · " + formatMinutes(line.minutes);
      track.append(seg);
      bits.push(line.title + " " + formatMinutes(line.minutes));
    });
    if (result.leftMinutes > 0) {
      const rest = document.createElement("div");
      rest.className = "seg rest";
      rest.style.flexGrow = String(result.leftMinutes);
      if (result.leftMinutes / freeMinutes >= 0.18) {
        const label = document.createElement("span");
        label.textContent = formatMinutes(result.leftMinutes);
        rest.append(label);
      }
      track.append(rest);
      bits.push(formatMinutes(result.leftMinutes) + " left");
    }
    bar.setAttribute("role", "img");
    bar.setAttribute("aria-label", bits.join(", ") || "Empty window");
  }

  async function fillPlan(animateLines) {
    const time = readTime(form);
    if (!time) {
      showError("Enter free time within seven days.");
      return;
    }
    const total = time.hours * 60 + time.minutes;
    try {
      const result = await api("/api/plan", { method: "POST", body: time });
      clearError();
      paintBar(result, total);
      plan.querySelector(".plan-total").textContent = "Window: " + formatMinutes(total);
      const lines = plan.querySelector(".plan-lines");
      lines.replaceChildren();
      (result.lines || []).forEach((line, index) => {
        const row = document.createElement("div");
        row.className = "plan-line";
        if (animateLines && !reduce) row.style.animationDelay = Math.min(index, 8) * 30 + "ms";
        row.innerHTML = '<span class="name"></span><span class="outcome"></span>';
        const match = items.find((item) => item.name === line.title);
        if (match) row.prepend(posterNode(match, "poster-sm"));
        row.querySelector(".name").textContent = line.title;
        const outcome = row.querySelector(".outcome");
        outcome.textContent = outcomeText(line);
        outcome.classList.add(line.outcome);
        lines.append(row);
      });
      const planLines = result.lines || [];
      const last = planLines[planLines.length - 1];
      const note = plan.querySelector(".plan-note");
      if (!last) note.textContent = "Nothing in the queue to lay out yet.";
      else if (result.leftMinutes === 0 && last.outcome === "full") note.textContent = "The window is full.";
      else if (result.leftMinutes === 0) note.textContent = "The window is full. Nothing else fits in it.";
      else if (last.outcome === "partial" || last.outcome === "stopped") {
        note.textContent = formatMinutes(result.leftMinutes) + " left. Nothing else fits in this window.";
      } else note.textContent = formatMinutes(result.leftMinutes) + " left.";
      plan.classList.add("open");
    } catch (err) {
      if (!gone(err)) showError(err.message);
    }
  }

  rhythmForm.addEventListener("input", () => {
    clearTimeout(paceTimer);
    const value = readDaily(rhythmForm);
    if (value == null) {
      showRhythm("Enter 0 to 1440 minutes a day.");
      return;
    }
    dailyMinutes = value;
    showRhythm(items.length ? rhythmParts(items, dailyMinutes) : "");
    paceTimer = setTimeout(() => savePace(value), 400);
  });

  rhythmForm.addEventListener("submit", (event) => {
    event.preventDefault();
    clearTimeout(paceTimer);
    const value = readDaily(rhythmForm);
    if (value == null) {
      showRhythm("Enter 0 to 1440 minutes a day.");
      return;
    }
    dailyMinutes = value;
    showRhythm(items.length ? rhythmParts(items, dailyMinutes) : "");
    savePace(value);
  });

  form.addEventListener("input", () => {
    summary.textContent = items.length ? summaryText(items) : "";
    clearTimeout(planTimer);
    if (plan.classList.contains("open") && readFree(form) > 0) {
      planTimer = setTimeout(() => fillPlan(false), 250);
    }
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = form.querySelector("button");
    setBusy(button, true);
    await fillPlan(true);
    setBusy(button, false);
  });

  list.addEventListener("click", async (event) => {
    const button = event.target.closest("[data-dir]");
    if (!button || button.disabled || busy) return;
    const row = button.closest(".q-item");
    if (button.dataset.dir === "remove") {
      remove(row.dataset.id, row);
      return;
    }
    const direction = button.dataset.dir === "up" ? "up" : "down";
    busy = true;
    const first = new Map();
    [...list.children].forEach((node) => first.set(node.dataset.id, node.getBoundingClientRect()));
    try {
      await shift(row.dataset.id, direction, 1);
      render();
      flip(first);
      if (plan.classList.contains("open")) fillPlan(false);
    } catch (err) {
      if (!gone(err)) showError(err.message);
    } finally {
      busy = false;
    }
  });

  list.addEventListener("pointerdown", (event) => {
    if (busy) return;
    const grip = event.target.closest(".grip");
    if (!grip) return;
    event.preventDefault();
    const row = grip.closest(".q-item");
    const origin = [...list.children].map((node) => {
      const box = node.getBoundingClientRect();
      return { id: node.dataset.id, top: box.top, height: box.height, mid: box.top + box.height / 2 };
    });
    drag = {
      id: row.dataset.id,
      y: event.clientY,
      active: false,
      pointer: event.pointerId,
      origin,
      target: origin.findIndex((item) => item.id === row.dataset.id),
    };
  });

  document.addEventListener("pointermove", (event) => {
    if (!drag || event.pointerId !== drag.pointer) return;
    const dy = event.clientY - drag.y;
    if (!drag.active && Math.abs(dy) < 6) return;
    drag.active = true;
    const home = drag.origin.find((item) => item.id === drag.id);
    const from = drag.origin.findIndex((item) => item.id === drag.id);
    const pointerMid = home.mid + dy;
    let target = 0;
    drag.origin.forEach((item) => {
      if (item.id !== drag.id && item.mid < pointerMid) target += 1;
    });
    drag.target = target;
    const stride = home.height + 8;
    [...list.children].forEach((node) => {
      const index = drag.origin.findIndex((item) => item.id === node.dataset.id);
      if (node.dataset.id === drag.id) {
        node.style.transform = "translateY(" + dy + "px)";
        node.style.zIndex = "2";
        node.classList.add("lifting");
        return;
      }
      let shiftY = 0;
      if (target > from && index > from && index <= target) shiftY = -stride;
      if (target < from && index < from && index >= target) shiftY = stride;
      node.style.transform = shiftY ? "translateY(" + shiftY + "px)" : "";
    });
  });

  function dropLift() {
    [...list.children].forEach((node) => {
      node.style.transform = "";
      node.style.zIndex = "";
      node.classList.remove("lifting");
    });
  }

  async function endDrag(event) {
    if (!drag || (event && event.pointerId !== drag.pointer)) return;
    const snapshot = drag;
    drag = null;
    if (!snapshot.active) {
      dropLift();
      return;
    }
    const from = snapshot.origin.findIndex((item) => item.id === snapshot.id);
    const steps = snapshot.target - from;
    if (!steps) {
      dropLift();
      return;
    }
    busy = true;
    const first = new Map();
    [...list.children].forEach((node) => first.set(node.dataset.id, node.getBoundingClientRect()));
    try {
      await shift(snapshot.id, steps > 0 ? "down" : "up", Math.abs(steps));
      render();
      flip(first);
      if (plan.classList.contains("open")) fillPlan(false);
    } catch (err) {
      dropLift();
      if (!gone(err)) showError(err.message);
      try { await reload(); } catch { /* shown above */ }
    } finally {
      busy = false;
    }
  }

  document.addEventListener("pointerup", endDrag);
  document.addEventListener("pointercancel", endDrag);

  document.querySelector(".empty-gate").addEventListener("click", async (event) => {
    const add = event.target.closest("[data-add]");
    if (add) {
      if (busy) return;
      busy = true;
      try {
        await api("/api/queue", { method: "POST", body: { titleId: Number(add.dataset.add) } });
        await reload();
      } catch (err) {
        if (!gone(err)) {
          showError(err.message);
          shelfError.hidden = false;
          shelfError.textContent = err.message;
        }
      } finally {
        busy = false;
      }
      return;
    }
    const card = event.target.closest("[data-shelf]");
    if (!card) return;
    openShelf = openShelf === card.dataset.shelf ? "" : card.dataset.shelf;
    paintShelfDetail();
  });

  document.querySelector("#undo").addEventListener("click", async () => {
    if (!undone || busy) return;
    const snapshot = undone;
    hideUndo();
    busy = true;
    try {
      const created = await api("/api/queue", { method: "POST", body: snapshot.seasonId ? { seasonId: snapshot.seasonId } : { titleId: snapshot.titleId } });
      const steps = created.position - 1 - snapshot.index;
      if (steps > 0) await shift(created.id, "up", steps);
      else await reload();
      render();
      if (plan.classList.contains("open")) fillPlan(false);
    } catch (err) {
      if (!gone(err)) showError(err.message);
      try { await reload(); } catch { /* shown above */ }
    } finally {
      busy = false;
    }
  });

  reload().catch((err) => {
    if (gone(err)) return;
    showError(err.message);
    shelfError.hidden = false;
    shelfError.textContent = err.message;
  });
}

async function mapLimit(list, limit, fn) {
  const out = new Array(list.length);
  let next = 0;
  async function worker() {
    while (next < list.length) {
      const index = next;
      next += 1;
      out[index] = await fn(list[index]);
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, list.length) }, worker));
  return out;
}

async function fetchFeatured() {
  let failed = null;
  const cards = await mapLimit(featuredQueries, 2, async (query) => {
    if (failed) return null;
    try {
      const data = await api("/api/titles?q=" + encodeURIComponent(query));
      const titles = data.titles || [];
      const exact = titles.find((card) => card.name.toLowerCase() === query.toLowerCase());
      const picked = exact || titles[0];
      return picked ? mapCard(picked) : null;
    } catch (err) {
      if (err.code === "CATALOG_UNAVAILABLE") failed = err;
      return null;
    }
  });
  if (failed) throw failed;
  return cards.filter(Boolean);
}

async function boot() {
  const page = document.body.dataset.page;
  if (page === "login" || page === "register") {
    try {
      await api("/api/me");
      location.replace("queue.html");
      return;
    } catch (err) {
      if (err.status !== 401) showError(err.message);
    }
    bindAuth(page);
    return;
  }
  if (page === "forgot") {
    bindForgot();
    return;
  }
  if (page === "reset") {
    bindReset();
    return;
  }
  let user;
  try {
    user = await api("/api/me");
  } catch (err) {
    if (err.status === 401) {
      location.replace("login.html");
      return;
    }
    showError(err.message);
    return;
  }
  paintChrome(user);
  if (page === "search") bindSearch();
  if (page === "queue") {
    bindAccountMail(user);
    bindQueue();
  }
  if (page === "title") bindTitle();
}

function bindTitle() {
  const params = new URLSearchParams(location.search);
  const id = params.get("id");
  const head = document.querySelector(".title-head");
  const seasons = document.querySelector(".seasons");
  if (!id) {
    showError("Could not read the request.");
    return;
  }

  let timer = 0;
  let tries = 0;

  async function load() {
    clearTimeout(timer);
    const [card, queue] = await Promise.all([api("/api/titles/" + id), api("/api/queue")]);
    const queuedSeasons = new Set((queue.items || []).map((item) => item.seasonId));
    head.replaceChildren();
    const poster = document.createElement("div");
    poster.append(posterNode(mapCard(card), "poster-lg"));
    const copy = document.createElement("div");
    const title = document.createElement("h1");
    title.textContent = card.name;
    const facts = document.createElement("p");
    facts.className = "facts";
    facts.textContent = (card.year || "") + " · " + runtimeLine(mapCard(card));
    copy.append(title, facts);
    if (card.summary) {
      const about = document.createElement("p");
      about.className = "summary-text";
      about.textContent = card.summary;
      copy.append(about);
    }
    if (card.assembling) {
      const note = document.createElement("p");
      note.className = "assembling";
      note.textContent = "Still assembling. Episodes will show up on their own.";
      copy.append(note);
      if (tries < 20) {
        tries += 1;
        timer = setTimeout(() => {
          load().catch((err) => {
            if (!gone(err)) showError(err.message);
          });
        }, 1500);
      }
    } else {
      tries = 0;
    }
    head.append(poster, copy);

    seasons.replaceChildren();
    const seasonList = card.seasons || [];
    if (seasonList.length > 1 && !card.assembling) {
      const row = document.createElement("div");
      row.className = "whole-row";
      const missing = seasonList.some((season) => !queuedSeasons.has(season.id));
      if (!missing) {
        const mark = document.createElement("span");
        mark.className = "queued";
        mark.textContent = "The whole series is in the queue";
        row.append(mark);
      } else {
        const button = document.createElement("button");
        button.className = "primary";
        button.type = "button";
        button.textContent = "Add the whole series";
        button.addEventListener("click", async () => {
          clearError();
          try {
            await api("/api/queue", { method: "POST", body: { titleId: Number(id), whole: true } });
            await load();
          } catch (err) {
            if (gone(err)) return;
            showError(err.message);
          }
        });
        row.append(button);
      }
      seasons.append(row);
    }
    seasonList.forEach((season) => {
      const block = document.createElement("section");
      block.className = "season";
      block.id = "season-" + season.number;
      const bar = document.createElement("div");
      bar.className = "season-head";
      const heading = document.createElement("h2");
      const watched = (season.episodes || []).filter((episode) => episode.watched).length;
      const total = (season.episodes || []).length;
      heading.textContent = "Season " + season.number + " · " + watched + " of " + total;
      const label = document.createElement("div");
      label.className = "season-label";
      if (season.poster) {
        const img = document.createElement("img");
        img.className = "season-poster";
        img.alt = "";
        img.src = season.poster;
        label.append(img);
      }
      label.append(heading);
      let action;
      if (queuedSeasons.has(season.id)) {
        action = document.createElement("span");
        action.className = "queued";
        action.textContent = "In the queue";
      } else {
        action = document.createElement("button");
        action.className = "primary";
        action.type = "button";
        action.textContent = "Add season";
        action.addEventListener("click", async () => {
          clearError();
          try {
            await api("/api/queue", { method: "POST", body: { seasonId: season.id } });
            action.replaceWith(Object.assign(document.createElement("span"), { className: "queued", textContent: "In the queue" }));
          } catch (err) {
            if (gone(err)) return;
            showError(err.message);
          }
        });
      }
      const episodes = season.episodes || [];
      const tools = document.createElement("div");
      tools.className = "season-tools";
      if (total > 0) {
        const all = watched === total;
        const sweep = document.createElement("button");
        sweep.className = "text-btn";
        sweep.type = "button";
        sweep.textContent = all ? "Clear season" : "Season watched";
        sweep.addEventListener("click", () => markMany(sweep, episodes.map((episode) => episode.id), !all));
        tools.append(sweep);
      }
      tools.append(action);
      bar.append(label, tools);
      block.append(bar);
      episodes.forEach((episode, index) => {
        const row = document.createElement("div");
        row.className = "episode";
        const name = document.createElement("span");
        name.className = "name" + (episode.watched ? " watched" : "");
        name.textContent = episode.number + ". " + (episode.name || "Episode") + (episode.runtime ? " · " + episode.runtime + " min" : "");
        const mark = document.createElement("button");
        mark.className = "ghost";
        mark.type = "button";
        mark.textContent = episode.watched ? "Watched" : "Not watched";
        mark.addEventListener("click", async () => {
          clearError();
          try {
            await api("/api/episodes/" + episode.id, { method: "POST", body: { watched: !episode.watched } });
            await load();
          } catch (err) {
            if (!gone(err)) showError(err.message);
          }
        });
        const before = episodes.slice(0, index + 1);
        if (index > 0 && before.some((item) => !item.watched)) {
          const upTo = document.createElement("button");
          upTo.className = "text-btn up-to";
          upTo.type = "button";
          upTo.textContent = "Watched up to here";
          upTo.addEventListener("click", () => markMany(upTo, before.map((item) => item.id), true));
          row.append(name, upTo, mark);
        } else {
          row.append(name, mark);
        }
        block.append(row);
      });
      seasons.append(block);
    });
  }

  async function markMany(button, ids, watched) {
    clearError();
    const label = button.textContent;
    button.disabled = true;
    button.textContent = "Saving…";
    try {
      await api("/api/episodes/watched", { method: "POST", body: { ids, watched } });
      await load();
    } catch (err) {
      button.disabled = false;
      button.textContent = label;
      if (!gone(err)) showError(err.message);
    }
  }

  load().then(() => {
    const target = params.get("season") && document.getElementById("season-" + params.get("season"));
    if (target) target.scrollIntoView({ block: "start" });
  }).catch((err) => {
    if (!gone(err)) showError(err.message);
  });
}

boot();
