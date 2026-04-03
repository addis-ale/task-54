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
          swapTarget(targetSelector, "<div class='card' style='border-left:4px solid var(--warn,#b04f2d);padding:1rem'><h4>Record Changed</h4><p>This record was modified by another user. The latest version has been reloaded.</p><button onclick=\"this.closest('.card').remove()\" style='margin-top:.5rem'>Dismiss</button></div>" + res.text, swapMode);
          bindAll();
          return;
        }
        if (res.status === 401) { window.location.href = "/login"; return; }
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
      button.dataset.originalText = button.textContent;
      button.textContent = "Submitting...";
    }
    request(method, url, target, swap, body);
    if (button) {
      setTimeout(function () {
        button.disabled = false;
        button.textContent = button.dataset.originalText || "Submit";
      }, 800);
    }
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
    Object.keys(localStorage).forEach(function (key) {
      if (key.indexOf("clinic:lru:") === 0) {
        localStorage.removeItem(key);
      }
    });
    if (window.clinicCache && typeof window.clinicCache.render === "function") {
      window.clinicCache.render();
    }
  };

  bindAll();
})();
