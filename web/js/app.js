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
    throw new Error("Не получилось связаться с программой.");
  }
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error((data.error && data.error.message) || "Что-то пошло не так. Попробуйте ещё раз.");
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
    about: "",
  };
}

function mapItem(item) {
  const card = mapCard(item);
  card.id = item.id;
  card.titleId = item.titleId;
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
  return item.kind === "film" ? "фильм" : "сериал";
}

function plural(n, one, few, many) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

function formatMinutes(total) {
  if (total < 60) return total + " мин";
  const hours = Math.floor(total / 60);
  const minutes = total % 60;
  return minutes ? hours + " ч " + minutes + " мин" : hours + " ч";
}

function seriesWord(n) {
  return plural(n, "серия", "серии", "серий");
}

function runtimeLine(item) {
  if (item.kind === "film") {
    if (!item.average) return "длительность неизвестна";
    return formatMinutes(item.average);
  }
  return item.episodes + " " + seriesWord(item.episodes) + ", средняя " + item.average + " мин";
}

function mountDetail(root, item, queued) {
  root.innerHTML = `
    <div class="detail">
      <div class="detail-poster"></div>
      <div class="detail-copy">
        <p class="facts"></p>
        <p class="blurb"></p>
        ${queued ? '<span class="queued">В очереди</span>' : '<button class="primary" type="button" data-add="">В очередь</button>'}
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
  if (line.outcome === "full") return "целиком · " + formatMinutes(line.minutes);
  if (line.outcome === "partial") return line.episodes + " " + seriesWord(line.episodes) + " · " + formatMinutes(line.minutes);
  if (line.outcome === "stopped") return "не влезает";
  return "длительность неизвестна";
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
      showError("Имя: от 3 до 32 символов, только буквы, цифры и подчёркивание.");
      return;
    }
    if (Array.from(password).length < 8) {
      showError("Пароль: не короче 8 символов.");
      return;
    }
    const button = form.querySelector("button");
    setBusy(button, true);
    try {
      if (mode === "register") {
        await api("/api/register", { method: "POST", body: { username, password } });
      }
      await api("/api/login", { method: "POST", body: { username, password } });
      location.href = "queue.html";
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

  function markQueued(button) {
    const mark = document.createElement("span");
    mark.className = "queued";
    mark.textContent = "В очереди";
    button.replaceWith(mark);
  }

  async function runSearch(fromSubmit) {
    const query = form.q.value.trim();
    const token = ++seq;
    if (Array.from(query).length < 2) {
      hits.replaceChildren();
      openId = "";
      if (fromSubmit) {
        hint.hidden = true;
        showError("Введите не меньше двух символов.");
      } else {
        clearError();
        hint.hidden = false;
        hint.textContent = "Введите название";
      }
      return;
    }
    clearError();
    hint.hidden = true;
    showSkeletons();
    const button = form.querySelector("button");
    if (fromSubmit) setBusy(button, true);
    try {
      const data = await api("/api/titles?q=" + encodeURIComponent(query));
      if (token !== seq) return;
      hits.replaceChildren();
      openId = "";
      (data.titles || []).forEach((card, index) => {
        const item = mapCard(card);
        hits.append(renderHit(item, queued.has(item.titleId), index));
      });
    } catch (err) {
      if (token !== seq) return;
      hits.replaceChildren();
      if (!gone(err)) showError(err.message);
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
    hit.querySelector(".hit-meta").textContent = item.year + " · " + kindOf(item);
    mountDetail(hit.querySelector(".fold-inner"), item, isQueued);
    return hit;
  }
}

function bindQueue() {
  const page = document.querySelector(".page");
  const list = document.querySelector(".queue");
  const plan = document.querySelector(".plan");
  const form = document.querySelector(".time");
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

  function summaryText(rows) {
    const minutes = rows.reduce((sum, item) => sum + (known(item) ? item.episodes * item.average : 0), 0);
    const unknown = rows.filter((item) => !known(item)).length;
    let text = rows.length + " " + plural(rows.length, "название", "названия", "названий");
    if (minutes > 0) text += " · " + formatMinutes(minutes);
    if (unknown > 0) text += " · " + unknown + " без длительности";
    const free = readFree(form);
    if (free > 0 && minutes > 0) {
      const ratio = minutes / free;
      if (ratio < 0.5) text += " · короче одного такого вечера";
      else {
        const evenings = Math.max(1, Math.round(ratio));
        text += evenings === 1
          ? " · около одного такого вечера"
          : " · около " + evenings + " таких вечеров";
      }
    }
    return text;
  }

  function render(skipFlipId) {
    page.classList.toggle("is-empty", items.length === 0);
    summary.textContent = items.length ? summaryText(items) : "";
    list.replaceChildren();
    items.forEach((item, index) => {
      const row = document.createElement("li");
      row.className = "q-item";
      if (!known(item)) row.classList.add("unknown");
      if (String(item.id) === String(skipFlipId)) row.classList.add("lifting");
      row.dataset.id = item.id;
      const total = item.episodes * item.average;
      const length = known(item) ? formatMinutes(total) : "длительность неизвестна";
      row.innerHTML = `
        <button class="grip" type="button" aria-label="Перетащить"><span></span></button>
        <div>
          <div class="hit-name"></div>
          <div class="hit-meta"></div>
        </div>
        <div class="q-actions">
          <button class="ghost" type="button" data-dir="up">выше</button>
          <button class="ghost" type="button" data-dir="down">ниже</button>
          <button class="ghost danger" type="button" data-dir="remove">убрать</button>
        </div>`;
      row.querySelector(".grip").after(posterNode(item));
      row.querySelector(".hit-name").textContent = item.name;
      row.querySelector(".hit-meta").textContent = (index + 1) + " · " + kindOf(item) + " · " + length;
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
    render();
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
    undone = { titleId: item.titleId, index, name: item.name };
    const toast = document.querySelector(".undo");
    toast.querySelector("p").textContent = "Убрали «" + item.name + "»";
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
      bits.push("останется " + formatMinutes(result.leftMinutes));
    }
    bar.setAttribute("role", "img");
    bar.setAttribute("aria-label", bits.join(", ") || "Пустое окно");
  }

  async function fillPlan(animateLines) {
    const time = readTime(form);
    if (!time) {
      showError("Укажите свободное время в пределах семи суток.");
      return;
    }
    const total = time.hours * 60 + time.minutes;
    try {
      const result = await api("/api/plan", { method: "POST", body: time });
      clearError();
      paintBar(result, total);
      plan.querySelector(".plan-total").textContent = "Окно: " + formatMinutes(total);
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
      if (!last) note.textContent = "В очереди пока нечего раскладывать по минутам.";
      else if (result.leftMinutes === 0 && last.outcome === "full") note.textContent = "Окно заполнено.";
      else if (result.leftMinutes === 0) note.textContent = "Окно заполнено. Дальше в это окно не берём.";
      else if (last.outcome === "partial" || last.outcome === "stopped") {
        note.textContent = "Останется " + formatMinutes(result.leftMinutes) + ". Дальше в это окно не берём.";
      } else note.textContent = "Останется " + formatMinutes(result.leftMinutes) + ".";
      plan.classList.add("open");
    } catch (err) {
      if (!gone(err)) showError(err.message);
    }
  }

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
      const created = await api("/api/queue", { method: "POST", body: { titleId: snapshot.titleId } });
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
  if (page === "queue") bindQueue();
}

boot();
