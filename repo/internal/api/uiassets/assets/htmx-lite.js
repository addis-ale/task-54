(function () {
  function idempotencyKey(url) {
    return (
      "idem:" +
      url +
      ":" +
      Date.now() +
      ":" +
      Math.random().toString(16).slice(2)
    );
  }

  function showIndicator(active) {
    var el = document.getElementById("htmx-indicator");
    if (!el) {
      return;
    }
    el.style.display = active ? "block" : "none";
  }

  function swapTarget(targetSelector, html, swapMode) {
    var target = document.querySelector(targetSelector || "#panel");
    if (!target) {
      return;
    }
    if (swapMode === "outerHTML") {
      target.outerHTML = html;
      return;
    }
    target.innerHTML = html;
  }

  function getCSRFToken() {
    var match = document.cookie.match(/(?:^|;\s*)clinic_csrf=([^;]*)/);
    return match ? match[1] : "";
  }

  function request(method, url, targetSelector, swapMode, body) {
    var headers = { "HX-Request": "true" };
    if (method !== "GET") {
      headers["Idempotency-Key"] = idempotencyKey(url);
      var csrf = getCSRFToken();
      if (csrf) { headers["X-CSRF-Token"] = csrf; }
    }

    var options = {
      method: method,
      credentials: "include",
      headers: headers,
    };
    if (body) {
      options.body = body;
    }

    showIndicator(true);
    fetch(url, options)
      .then(function (res) {
        return res.text().then(function (text) {
          return { status: res.status, text: text };
        });
      })
      .then(function (res) {
        if (res.status === 409) {
          // Show conflict banner then reload the current panel to get latest state
          var banner = "<div class='card' style='border-left:4px solid var(--warn,#b04f2d);padding:1rem;margin-bottom:1rem'><h4>Record Changed</h4><p>This record was modified by another user. The latest version has been reloaded below.</p><button onclick=\"this.closest('.card').remove()\" style='margin-top:.5rem'>Dismiss</button></div>";
          var activeNav = document.querySelector('.top-nav button[data-active]');
          var reloadUrl = activeNav ? activeNav.getAttribute('hx-get') : null;
          if (reloadUrl) {
            // Fetch latest panel state and show it with the conflict banner
            fetch(reloadUrl, { method: 'GET', credentials: 'include', headers: { 'HX-Request': 'true' } })
              .then(function (r) { return r.text(); })
              .then(function (freshHtml) {
                swapTarget(targetSelector, banner + freshHtml, swapMode);
                bindAll();
              })
              .catch(function () {
                swapTarget(targetSelector, banner + res.text, swapMode);
                bindAll();
              });
          } else {
            // Fallback: show response body (server-rendered latest state on conflict)
            swapTarget(targetSelector, banner + res.text, swapMode);
            bindAll();
          }
          return;
        }
        if (res.status === 401) { window.location.href = "/login"; return; }
        autoCacheExerciseContent(url, res.text);
        swapTarget(targetSelector, res.text, swapMode);
        bindAll();
      })
      .catch(function () {
        swapTarget(
          targetSelector,
          "<div class='card'>Request failed</div>",
          swapMode,
        );
      })
      .finally(function () {
        showIndicator(false);
      });
  }

  function submitForm(form) {
    if (form.dataset.htmxInflight === "1") { return; }
    form.dataset.htmxInflight = "1";
    var url =
      form.getAttribute("hx-post") ||
      form.getAttribute("action") ||
      window.location.pathname;
    var method = form.getAttribute("hx-post")
      ? "POST"
      : (form.method || "POST").toUpperCase();
    var target = form.getAttribute("hx-target") || "#panel";
    var swap = form.getAttribute("hx-swap") || "innerHTML";
    var body = new FormData(form);
    var button = form.querySelector("button[type='submit']");
    if (button) {
      button.disabled = true;
      button.dataset.originalText = button.dataset.originalText || button.textContent;
      button.textContent = "Submitting...";
    }

    var headers = { "HX-Request": "true" };
    if (!form.dataset.htmxIdemKey) {
      form.dataset.htmxIdemKey = idempotencyKey(url);
    }
    headers["Idempotency-Key"] = form.dataset.htmxIdemKey;
    var csrf = getCSRFToken();
    if (csrf) { headers["X-CSRF-Token"] = csrf; }

    showIndicator(true);
    fetch(url, { method: method, credentials: "include", headers: headers, body: body })
      .then(function (res) {
        return res.text().then(function (text) {
          return { status: res.status, text: text };
        });
      })
      .then(function (res) {
        delete form.dataset.htmxIdemKey;
        if (res.status === 409) {
          var banner = "<div class='card' style='border-left:4px solid var(--warn,#b04f2d);padding:1rem;margin-bottom:1rem'><h4>Record Changed</h4><p>This record was modified by another user. The latest version has been reloaded below.</p><button onclick=\"this.closest('.card').remove()\" style='margin-top:.5rem'>Dismiss</button></div>";
          var activeNav = document.querySelector('.top-nav button[data-active]');
          var reloadUrl = activeNav ? activeNav.getAttribute('hx-get') : null;
          if (reloadUrl) {
            fetch(reloadUrl, { method: 'GET', credentials: 'include', headers: { 'HX-Request': 'true' } })
              .then(function (r) { return r.text(); })
              .then(function (freshHtml) {
                swapTarget(target, banner + freshHtml, swap);
                bindAll();
              })
              .catch(function () {
                swapTarget(target, banner + res.text, swap);
                bindAll();
              });
          } else {
            swapTarget(target, banner + res.text, swap);
            bindAll();
          }
          return;
        }
        if (res.status === 401) { window.location.href = "/login"; return; }
        swapTarget(target, res.text, swap);
        bindAll();
      })
      .catch(function () {
        swapTarget(target, "<div class='card'>Request failed</div>", swap);
      })
      .finally(function () {
        showIndicator(false);
        form.dataset.htmxInflight = "0";
        if (button) {
          button.disabled = false;
          button.textContent = button.dataset.originalText || "Submit";
        }
      });
  }

  function bindAll() {
    Array.prototype.slice
      .call(document.querySelectorAll("[hx-get]"))
      .forEach(function (el) {
        if (el.dataset.htmxBoundGet === "1") {
          return;
        }
        el.dataset.htmxBoundGet = "1";
        var trigger = el.getAttribute("hx-trigger") || "click";
        var fn = function (event) {
          if (event) {
            event.preventDefault();
          }
          request(
            "GET",
            el.getAttribute("hx-get"),
            el.getAttribute("hx-target"),
            el.getAttribute("hx-swap") || "innerHTML",
          );
        };
        if (trigger === "load") {
          setTimeout(fn, 0);
        } else {
          el.addEventListener(trigger, fn);
        }
      });

    Array.prototype.slice
      .call(document.querySelectorAll("[hx-post]"))
      .forEach(function (el) {
        if (el.dataset.htmxBoundPost === "1") {
          return;
        }
        el.dataset.htmxBoundPost = "1";
        if (el.tagName === "FORM") {
          el.addEventListener("submit", function (event) {
            event.preventDefault();
            submitForm(el);
          });
        } else {
          el.addEventListener("click", function (event) {
            event.preventDefault();
            request(
              "POST",
              el.getAttribute("hx-post"),
              el.getAttribute("hx-target") || "#panel",
              el.getAttribute("hx-swap") || "innerHTML",
              null,
            );
          });
        }
      });
  }

  window.clearClinicDeviceCache = function () {
    // Clear localStorage cache items
    Object.keys(localStorage).forEach(function (key) {
      if (key.indexOf("clinic:lru:") === 0) {
        localStorage.removeItem(key);
      }
    });
    // Clear IndexedDB cache (clinic_lru_cache database)
    if (window.clinicCache && typeof window.clinicCache.clearAll === "function") {
      window.clinicCache.clearAll();
    } else {
      try {
        var delReq = indexedDB.deleteDatabase("clinic_lru_cache");
        delReq.onsuccess = function () {};
        delReq.onerror = function () {};
      } catch (e) {}
    }
  };

  // ---- Lightweight IndexedDB LRU cache (initialized at app boot) ----
  (function initClinicCache() {
    var DB_NAME = "clinic_lru_cache";
    var STORE = "cache_items";
    var MAX_BYTES = 2 * 1024 * 1024 * 1024;
    var MAX_ITEMS = 200;

    function openDB() {
      return new Promise(function (resolve, reject) {
        var req = indexedDB.open(DB_NAME, 1);
        req.onupgradeneeded = function (e) {
          var db = e.target.result;
          if (!db.objectStoreNames.contains(STORE)) {
            var s = db.createObjectStore(STORE, { keyPath: "key" });
            s.createIndex("last_accessed_at", "last_accessed_at", { unique: false });
          }
        };
        req.onsuccess = function (e) { resolve(e.target.result); };
        req.onerror = function (e) { reject(e.target.error); };
      });
    }

    function all(db) {
      return new Promise(function (resolve, reject) {
        var tx = db.transaction(STORE, "readonly");
        var r = tx.objectStore(STORE).getAll();
        r.onsuccess = function () { resolve(r.result || []); };
        r.onerror = function () { reject(r.error); };
      });
    }

    function putDB(db, item) {
      return new Promise(function (resolve, reject) {
        var tx = db.transaction(STORE, "readwrite");
        tx.objectStore(STORE).put(item);
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { reject(tx.error); };
      });
    }

    function delDB(db, key) {
      return new Promise(function (resolve, reject) {
        var tx = db.transaction(STORE, "readwrite");
        tx.objectStore(STORE).delete(key);
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { reject(tx.error); };
      });
    }

    function clearDB(db) {
      return new Promise(function (resolve, reject) {
        var tx = db.transaction(STORE, "readwrite");
        tx.objectStore(STORE).clear();
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { reject(tx.error); };
      });
    }

    async function evict(db) {
      var items = await all(db);
      var total = items.reduce(function (s, i) { return s + (i.size_bytes || 0); }, 0);
      if (items.length <= MAX_ITEMS && total <= MAX_BYTES) return;
      items.sort(function (a, b) { return new Date(a.last_accessed_at || 0) - new Date(b.last_accessed_at || 0); });
      while (items.length > MAX_ITEMS || total > MAX_BYTES) {
        var v = items.shift();
        if (!v) break;
        total -= v.size_bytes || 0;
        await delDB(db, v.key);
      }
    }

    if (!window.clinicCache) {
      window.clinicCache = {
        put: async function (key, sizeBytes, hash, data) {
          var db = await openDB();
          await putDB(db, { key: key, size_bytes: sizeBytes, content_hash: hash, data: data || null, created_at: new Date().toISOString(), last_accessed_at: new Date().toISOString() });
          await evict(db);
        },
        get: async function (key) {
          var db = await openDB();
          return new Promise(function (resolve, reject) {
            var tx = db.transaction(STORE, "readwrite");
            var r = tx.objectStore(STORE).get(key);
            r.onsuccess = function () {
              var item = r.result;
              if (!item) { resolve(null); return; }
              item.last_accessed_at = new Date().toISOString();
              tx.objectStore(STORE).put(item);
              resolve(item);
            };
            r.onerror = function () { reject(r.error); };
          });
        },
        clearAll: async function () {
          var db = await openDB();
          await clearDB(db);
        },
        render: function () {}
      };
    }
  })();

  // Auto-cache recently viewed exercise content when navigating to exercise details or media
  function autoCacheExerciseContent(url, html) {
    if (!window.clinicCache || typeof window.clinicCache.put !== "function") return;
    var exerciseMatch = url.match(/\/exercises\/(\d+)/);
    var mediaMatch = url.match(/\/media\/(\d+)/);
    if (exerciseMatch) {
      var key = "exercise:" + exerciseMatch[1] + ":detail";
      var size = new Blob([html]).size;
      var hash = "sha256_" + Date.now();
      window.clinicCache.put(key, size, hash, html);
    }
    if (mediaMatch) {
      var mKey = "media:" + mediaMatch[1] + ":stream";
      var mSize = new Blob([html]).size;
      var mHash = "sha256_" + Date.now();
      window.clinicCache.put(mKey, mSize, mHash, html);
    }
  }

  bindAll();
})();
