package handler

import "github.com/gofiber/fiber/v2"

type CacheUIHandler struct{}

func NewCacheUIHandler() *CacheUIHandler {
	return &CacheUIHandler{}
}

// LRUSimulator renders a cache inspector UI that uses the shared window.clinicCache
// API (initialized at app boot in htmx-lite.js). This panel is a diagnostic/inspector
// tool — it does NOT initialize the cache, it only visualizes and manually tests it.
func (h *CacheUIHandler) LRUSimulator(c *fiber.Ctx) error {
	c.Type("html", "utf-8")
	return c.SendString(`
<section id="cache-lru-simulator" class="cache-simulator">
  <h3>Client LRU Cache Inspector</h3>
  <p>IndexedDB-backed cache for recently viewed exercise content and media. Limits: 2 GB total or 200 items.
     The cache is automatically populated when viewing exercises and media. Use the controls below to inspect or manually test entries.</p>
  <form id="cache-put-form" onsubmit="event.preventDefault(); if(window.clinicCache){var k=document.getElementById('cache-item-key').value;var s=document.getElementById('cache-item-size').value;var h=document.getElementById('cache-item-hash').value;window.clinicCache.put(k,Number(s),h).then(function(){window.clinicCache.render()});}">
    <label>Item Key <input type="text" id="cache-item-key" value="exercise:1:video" /></label>
    <label>Size (bytes) <input type="number" id="cache-item-size" value="1048576" min="1" /></label>
    <label>Hash <input type="text" id="cache-item-hash" value="sha256_demo" /></label>
    <button type="submit">Cache Item</button>
  </form>
  <button type="button" onclick="if(window.clinicCache)window.clinicCache.render()">Refresh View</button>
  <button type="button" class="warn" onclick="window.clearClinicDeviceCache()">Clear Device Cache</button>
  <pre id="cache-lru-output">Loading cache state...</pre>
  <script>
  // Render cache state using shared clinicCache API from htmx-lite.js
  (function() {
    function renderInspector() {
      var output = document.getElementById('cache-lru-output');
      if (!output || !window.clinicCache) {
        if (output) output.textContent = 'Cache API not yet initialized.';
        return;
      }
      // Use the shared openDB from clinicCache
      var DB_NAME = 'clinic_lru_cache';
      var STORE = 'cache_items';
      var MAX_BYTES = 2 * 1024 * 1024 * 1024;
      var MAX_ITEMS = 200;
      var req = indexedDB.open(DB_NAME, 1);
      req.onsuccess = function(e) {
        var db = e.target.result;
        if (!db.objectStoreNames.contains(STORE)) {
          output.textContent = JSON.stringify({limit_bytes:MAX_BYTES,limit_items:MAX_ITEMS,total_bytes:0,item_count:0,items:[]}, null, 2);
          return;
        }
        var tx = db.transaction(STORE, 'readonly');
        var getAll = tx.objectStore(STORE).getAll();
        getAll.onsuccess = function() {
          var items = getAll.result || [];
          items.sort(function(a,b){ return new Date(b.last_accessed_at||0)-new Date(a.last_accessed_at||0); });
          var totalBytes = items.reduce(function(s,i){ return s + (i.size_bytes||0); }, 0);
          var display = items.map(function(i){ return {key:i.key,size_bytes:i.size_bytes,content_hash:i.content_hash,created_at:i.created_at,last_accessed_at:i.last_accessed_at}; });
          output.textContent = JSON.stringify({limit_bytes:MAX_BYTES,limit_items:MAX_ITEMS,total_bytes:totalBytes,item_count:items.length,items:display}, null, 2);
        };
      };
      req.onerror = function() { output.textContent = 'Cache read error'; };
    }
    // Override render on shared cache to also update inspector
    if (window.clinicCache) {
      window.clinicCache.render = renderInspector;
    }
    renderInspector();
  })();
  </script>
</section>
`)
}
