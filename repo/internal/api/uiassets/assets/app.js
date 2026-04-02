(function () {
  const state = {
    user: null,
    permissions: [],
    scheduleTemplates: loadJSON("clinic:schedule:templates", []),
    favorites: loadJSON("clinic:exercise:favorites", {}),
  };

  const el = {
    loginPanel: document.getElementById("login-panel"),
    appPanel: document.getElementById("app-panel"),
    sessionChip: document.getElementById("session-chip"),
    activityLog: document.getElementById("activity-log"),
  };

  function loadJSON(key, fallback) {
    try {
      const raw = localStorage.getItem(key);
      return raw ? JSON.parse(raw) : fallback;
    } catch (_) {
      return fallback;
    }
  }

  function saveJSON(key, value) {
    localStorage.setItem(key, JSON.stringify(value));
  }

  function log(message, payload) {
    const now = new Date().toISOString();
    const line = payload
      ? `[${now}] ${message} ${JSON.stringify(payload, null, 2)}`
      : `[${now}] ${message}`;
    el.activityLog.textContent = line + "\n" + el.activityLog.textContent;
  }

  async function api(method, url, body, options) {
    const opts = options || {};
    const headers = Object.assign(
      { "Content-Type": "application/json" },
      opts.headers || {},
    );
    if (
      ["POST", "PUT", "PATCH", "DELETE"].includes(method.toUpperCase()) &&
      !headers["Idempotency-Key"]
    ) {
      headers["Idempotency-Key"] =
        `${method}:${url}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`;
    }

    const response = await fetch(url, {
      method,
      credentials: "include",
      headers,
      body: body == null ? undefined : JSON.stringify(body),
    });

    const type = response.headers.get("content-type") || "";
    if (type.includes("application/json")) {
      const payload = await response.json();
      if (!response.ok) {
        throw new Error(
          payload && payload.error
            ? payload.error.message
            : `HTTP ${response.status}`,
        );
      }
      return payload;
    }

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    return response;
  }

  function clearClinicLocalStorage() {
    Object.keys(localStorage)
      .filter(function (k) {
        return k.startsWith("clinic:");
      })
      .forEach(function (k) {
        localStorage.removeItem(k);
      });
  }

  function setSignedOut() {
    state.user = null;
    state.permissions = [];
    clearClinicLocalStorage();
    el.loginPanel.classList.remove("hidden");
    el.appPanel.classList.add("hidden");
    el.sessionChip.textContent = "Signed out";
  }

  function setSignedIn() {
    el.loginPanel.classList.add("hidden");
    el.appPanel.classList.remove("hidden");
    el.sessionChip.textContent = `${state.user.username} (${state.user.role})`;
    document.getElementById("permissions-output").textContent = JSON.stringify(
      state.permissions,
      null,
      2,
    );
  }

  async function refreshMe() {
    try {
      const payload = await api("GET", "/api/v1/auth/me");
      state.user = payload.data.user;
      state.permissions = payload.data.permissions || [];
      setSignedIn();
      await refreshOverview();
      await refreshOccupancy();
      await refreshExercises();
      await refreshPayments();
      await loadCacheFragment();
      renderTemplates();
    } catch (err) {
      setSignedOut();
      log("Session not available", { error: String(err) });
    }
  }

  async function login(event) {
    event.preventDefault();
    const username = document.getElementById("login-username").value.trim();
    const password = document.getElementById("login-password").value;
    try {
      await api("POST", "/api/v1/auth/login", { username, password });
      log("Login succeeded", { username });
      await refreshMe();
    } catch (err) {
      log("Login failed", { error: String(err) });
      alert(`Login failed: ${String(err)}`);
    }
  }

  function wireViewNav() {
    const buttons = Array.from(document.querySelectorAll(".top-nav button"));
    buttons.forEach((button) => {
      button.addEventListener("click", () => {
        buttons.forEach((b) => b.classList.remove("active"));
        button.classList.add("active");
        const view = button.getAttribute("data-view");
        Array.from(document.querySelectorAll(".view")).forEach((section) => {
          section.classList.toggle("active", section.id === `view-${view}`);
        });
      });
    });
  }

  async function refreshOverview() {
    try {
      const payload = await api("GET", "/api/v1/reports/ops/summary");
      document.getElementById("summary-output").textContent = JSON.stringify(
        payload.data.summary,
        null,
        2,
      );
    } catch (err) {
      document.getElementById("summary-output").textContent =
        `Failed to load summary: ${String(err)}`;
    }
  }

  async function refreshOccupancy() {
    try {
      const response = await fetch("/ui/occupancy/board", {
        credentials: "include",
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      document.getElementById("occupancy-board").innerHTML =
        await response.text();
    } catch (err) {
      document.getElementById("occupancy-board").textContent =
        `Failed to load board: ${String(err)}`;
    }
  }

  function renderExercises(items) {
    const host = document.getElementById("exercise-list");
    if (!items.length) {
      host.textContent = "No exercises yet.";
      return;
    }
    host.innerHTML = "";
    items.forEach((exercise) => {
      const row = document.createElement("div");
      row.className = "exercise-row";
      const left = document.createElement("div");
      left.innerHTML = `<strong>${exercise.title}</strong><div>${exercise.difficulty || "n/a"}</div>`;
      const button = document.createElement("button");
      const key = String(exercise.id);
      const favored = Boolean(state.favorites[key]);
      button.textContent = favored ? "Favorited" : "Add Favorite";
      if (favored) {
        button.classList.add("favorite");
      }
      button.addEventListener("click", () => {
        state.favorites[key] = !state.favorites[key];
        saveJSON("clinic:exercise:favorites", state.favorites);
        refreshExercises();
      });
      row.appendChild(left);
      row.appendChild(button);
      host.appendChild(row);
    });
  }

  async function cacheExerciseContent(exercises) {
    if (!window.clinicCache || !window.clinicCache.put) return;
    for (var i = 0; i < exercises.length; i++) {
      var ex = exercises[i];
      var key = "exercise:" + ex.id + ":detail";
      var json = JSON.stringify(ex);
      await window.clinicCache.put(key, json.length, "hash_" + (ex.version || ex.id), json);
    }
  }

  async function refreshExercises() {
    try {
      const payload = await api("GET", "/api/v1/exercises");
      var exercises = payload.data.exercises || [];
      renderExercises(exercises);
      cacheExerciseContent(exercises);
    } catch (err) {
      document.getElementById("exercise-list").textContent =
        `Failed to load exercises: ${String(err)}`;
    }
  }

  async function loadCacheFragment() {
    try {
      const response = await fetch("/ui/cache/lru", { credentials: "include" });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      document.getElementById("cache-fragment").innerHTML =
        await response.text();
      log("Loaded cache simulator fragment");
    } catch (err) {
      document.getElementById("cache-fragment").textContent =
        `Failed to load cache simulator: ${String(err)}`;
    }
  }

  function clearDeviceCache() {
    if (window.clinicCache && typeof window.clinicCache.clearAll === "function") {
      window.clinicCache.clearAll();
    }
    var keys = Object.keys(localStorage).filter(function (key) {
      return key.startsWith("clinic:lru:");
    });
    keys.forEach(function (key) {
      localStorage.removeItem(key);
    });
    log("Cleared device cache");
  }

  function formObject(form) {
    const data = new FormData(form);
    const out = {};
    data.forEach((value, key) => {
      out[key] = value;
    });
    return out;
  }

  function renderTemplates() {
    const host = document.getElementById("schedule-templates");
    host.innerHTML = "";
    if (!state.scheduleTemplates.length) {
      host.textContent = "No saved templates.";
      return;
    }
    state.scheduleTemplates.forEach((item, index) => {
      const row = document.createElement("div");
      row.className = "exercise-row";
      row.innerHTML = `<div><strong>${item.exam_id}</strong><div>${item.start_at} to ${item.end_at}</div></div>`;
      const useBtn = document.createElement("button");
      useBtn.textContent = "Load";
      useBtn.addEventListener("click", () => applyTemplate(index));
      row.appendChild(useBtn);
      host.appendChild(row);
    });
  }

  function applyTemplate(index) {
    const item = state.scheduleTemplates[index];
    if (!item) {
      return;
    }
    const form = document.getElementById("schedule-form");
    Object.keys(item).forEach((key) => {
      if (form.elements[key]) {
        form.elements[key].value = item[key];
      }
    });
  }

  function saveTemplate() {
    const form = document.getElementById("schedule-form");
    const payload = formObject(form);
    state.scheduleTemplates.unshift(payload);
    state.scheduleTemplates = state.scheduleTemplates.slice(0, 20);
    saveJSON("clinic:schedule:templates", state.scheduleTemplates);
    renderTemplates();
    log("Saved scheduling template", { exam_id: payload.exam_id });
  }

  async function publishTemplate() {
    const form = document.getElementById("schedule-form");
    const payload = formObject(form);
    payload.room_id = Number(payload.room_id);
    payload.proctor_id = Number(payload.proctor_id);
    payload.candidate_ids = String(payload.candidate_ids)
      .split(",")
      .map((x) => Number(x.trim()))
      .filter((x) => Number.isFinite(x) && x > 0);
    try {
      const response = await api("POST", "/api/v1/exam-schedules", payload);
      log("Published schedule", response.data.schedule);
      alert("Schedule published.");
    } catch (err) {
      log("Failed schedule publish", { error: String(err) });
      alert(`Failed to publish: ${String(err)}`);
    }
  }

  async function submitPayment(event) {
    event.preventDefault();
    const payload = formObject(event.target);
    payload.amount_cents = Number(payload.amount_cents);
    try {
      const response = await api("POST", "/api/v1/payments", payload);
      log("Payment created", response.data.payment);
      await refreshPayments();
    } catch (err) {
      log("Payment create failed", { error: String(err) });
      alert(`Payment failed: ${String(err)}`);
    }
  }

  async function submitRefund(event) {
    event.preventDefault();
    const payload = formObject(event.target);
    const paymentID = Number(payload.payment_id);
    try {
      const response = await api(
        "POST",
        `/api/v1/payments/${paymentID}/refunds`,
        {
          amount_cents: Number(payload.amount_cents),
          reason: payload.reason,
        },
      );
      log("Refund created", response.data.refund);
      await refreshPayments();
    } catch (err) {
      log("Refund failed", { error: String(err) });
      alert(`Refund failed: ${String(err)}`);
    }
  }

  async function submitSettlement(event) {
    event.preventDefault();
    const payload = formObject(event.target);
    payload.actual_total_cents = Number(payload.actual_total_cents);
    try {
      const response = await api("POST", "/api/v1/settlements/run", payload);
      log("Settlement completed", response.data.settlement);
      await refreshPayments();
    } catch (err) {
      log("Settlement failed", { error: String(err) });
      alert(`Settlement failed: ${String(err)}`);
    }
  }

  async function refreshPayments() {
    try {
      const payload = await api("GET", "/api/v1/payments");
      document.getElementById("payments-output").textContent = JSON.stringify(
        payload.data.payments || [],
        null,
        2,
      );
    } catch (err) {
      document.getElementById("payments-output").textContent =
        `Failed to load payments: ${String(err)}`;
    }
  }

  async function exportFinance(format) {
    try {
      const response = await api(
        "GET",
        `/api/v1/reports/finance/export?format=${encodeURIComponent(format)}`,
        null,
        { headers: {} },
      );
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download =
        format === "xlsx" ? "finance_report.xlsx" : "finance_report.csv";
      anchor.click();
      URL.revokeObjectURL(url);
      log("Downloaded finance export", { format });
    } catch (err) {
      log("Finance export failed", { format, error: String(err) });
      alert(`Export failed: ${String(err)}`);
    }
  }

  async function pingAudit() {
    try {
      const payload = await api("GET", "/api/v1/admin/audit/ping");
      document.getElementById("audit-output").textContent = JSON.stringify(
        payload.data,
        null,
        2,
      );
    } catch (err) {
      document.getElementById("audit-output").textContent =
        `Audit ping failed: ${String(err)}`;
    }
  }

  async function downloadDiagnostics() {
    try {
      const response = await api("POST", "/api/v1/diagnostics/export", {});
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = "diagnostics.zip";
      anchor.click();
      URL.revokeObjectURL(url);
      log("Downloaded diagnostics bundle");
    } catch (err) {
      log("Diagnostics export failed", { error: String(err) });
      alert(`Diagnostics export failed: ${String(err)}`);
    }
  }

  function wireUI() {
    wireViewNav();
    document.getElementById("login-form").addEventListener("submit", login);
    document
      .getElementById("refresh-summary")
      .addEventListener("click", refreshOverview);
    document
      .getElementById("refresh-occupancy")
      .addEventListener("click", refreshOccupancy);
    document
      .getElementById("refresh-exercises")
      .addEventListener("click", refreshExercises);
    document
      .getElementById("clear-device-cache")
      .addEventListener("click", clearDeviceCache);
    document
      .getElementById("save-template")
      .addEventListener("click", saveTemplate);
    document
      .getElementById("publish-template")
      .addEventListener("click", publishTemplate);
    document
      .getElementById("payment-form")
      .addEventListener("submit", submitPayment);
    document
      .getElementById("refund-form")
      .addEventListener("submit", submitRefund);
    document
      .getElementById("settlement-form")
      .addEventListener("submit", submitSettlement);
    document
      .getElementById("refresh-payments")
      .addEventListener("click", refreshPayments);
    document
      .getElementById("export-csv")
      .addEventListener("click", function () {
        exportFinance("csv");
      });
    document
      .getElementById("export-xlsx")
      .addEventListener("click", function () {
        exportFinance("xlsx");
      });
    document.getElementById("audit-ping").addEventListener("click", pingAudit);
    document
      .getElementById("download-diagnostics")
      .addEventListener("click", downloadDiagnostics);
  }

  window.clearClinicDeviceCache = clearDeviceCache;

  wireUI();
  refreshMe();
})();
