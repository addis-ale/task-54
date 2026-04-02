package handler

import "github.com/gofiber/fiber/v2"

type CacheUIHandler struct{}

func NewCacheUIHandler() *CacheUIHandler {
	return &CacheUIHandler{}
}

func (h *CacheUIHandler) LRUSimulator(c *fiber.Ctx) error {
	c.Type("html", "utf-8")
	return c.SendString(`
<section id="cache-lru-simulator" class="cache-simulator">
  <h3>Client LRU Cache</h3>
  <p>IndexedDB-backed cache for recently viewed exercise content and media. Limits: 2 GB total or 200 items.</p>
  <form id="cache-put-form" onsubmit="event.preventDefault(); window.clinicCache.putFromForm();">
    <label>Item Key <input type="text" id="cache-item-key" value="exercise:1:video" /></label>
    <label>Size (bytes) <input type="number" id="cache-item-size" value="1048576" min="1" /></label>
    <label>Hash <input type="text" id="cache-item-hash" value="sha256_demo" /></label>
    <button type="submit">Cache Item</button>
  </form>
  <button type="button" onclick="window.clinicCache.render()">Refresh View</button>
  <button type="button" class="warn" onclick="window.clinicCache.clearAll()">Clear Device Cache</button>
  <pre id="cache-lru-output"></pre>
  <script>
  (function() {
    var DB_NAME = 'clinic_lru_cache';
    var STORE_NAME = 'cache_items';
    var DB_VERSION = 1;
    var MAX_BYTES = 2 * 1024 * 1024 * 1024;
    var MAX_ITEMS = 200;

    function openDB() {
      return new Promise(function(resolve, reject) {
        var request = indexedDB.open(DB_NAME, DB_VERSION);
        request.onupgradeneeded = function(event) {
          var db = event.target.result;
          if (!db.objectStoreNames.contains(STORE_NAME)) {
            var store = db.createObjectStore(STORE_NAME, { keyPath: 'key' });
            store.createIndex('last_accessed_at', 'last_accessed_at', { unique: false });
          }
        };
        request.onsuccess = function(event) { resolve(event.target.result); };
        request.onerror = function(event) { reject(event.target.error); };
      });
    }

    function getAllItems(db) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readonly');
        var store = tx.objectStore(STORE_NAME);
        var request = store.getAll();
        request.onsuccess = function() { resolve(request.result || []); };
        request.onerror = function() { reject(request.error); };
      });
    }

    function putItemDB(db, item) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        var store = tx.objectStore(STORE_NAME);
        store.put(item);
        tx.oncomplete = function() { resolve(); };
        tx.onerror = function() { reject(tx.error); };
      });
    }

    function deleteItemDB(db, key) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        var store = tx.objectStore(STORE_NAME);
        store.delete(key);
        tx.oncomplete = function() { resolve(); };
        tx.onerror = function() { reject(tx.error); };
      });
    }

    function clearAllDB(db) {
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        var store = tx.objectStore(STORE_NAME);
        store.clear();
        tx.oncomplete = function() { resolve(); };
        tx.onerror = function() { reject(tx.error); };
      });
    }

    function nowISO() { return new Date().toISOString(); }

    async function evict(db) {
      var items = await getAllItems(db);
      var totalBytes = items.reduce(function(sum, i) { return sum + (i.size_bytes || 0); }, 0);
      if (items.length <= MAX_ITEMS && totalBytes <= MAX_BYTES) return;
      items.sort(function(a, b) {
        return new Date(a.last_accessed_at || 0).getTime() - new Date(b.last_accessed_at || 0).getTime();
      });
      while (items.length > MAX_ITEMS || totalBytes > MAX_BYTES) {
        var victim = items.shift();
        if (!victim) break;
        totalBytes -= (victim.size_bytes || 0);
        await deleteItemDB(db, victim.key);
      }
    }

    async function putItem(key, sizeBytes, hash, data) {
      var normalizedKey = String(key || '').trim();
      var normalizedHash = String(hash || '').trim();
      var normalizedSize = Math.max(1, Number(sizeBytes || 0));
      if (!normalizedKey || !normalizedHash || !Number.isFinite(normalizedSize)) return;

      var db = await openDB();
      var item = {
        key: normalizedKey,
        size_bytes: normalizedSize,
        content_hash: normalizedHash,
        data: data || null,
        created_at: nowISO(),
        last_accessed_at: nowISO()
      };
      await putItemDB(db, item);
      await evict(db);
    }

    async function getItem(key) {
      var db = await openDB();
      return new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readwrite');
        var store = tx.objectStore(STORE_NAME);
        var request = store.get(key);
        request.onsuccess = function() {
          var item = request.result;
          if (!item) { resolve(null); return; }
          item.last_accessed_at = nowISO();
          store.put(item);
          resolve(item);
        };
        request.onerror = function() { reject(request.error); };
      });
    }

    async function invalidateIfHashMismatch(key, expectedHash) {
      var db = await openDB();
      var item = await new Promise(function(resolve, reject) {
        var tx = db.transaction(STORE_NAME, 'readonly');
        var request = tx.objectStore(STORE_NAME).get(key);
        request.onsuccess = function() { resolve(request.result); };
        request.onerror = function() { reject(request.error); };
      });
      if (item && item.content_hash !== expectedHash) {
        await deleteItemDB(db, key);
      }
    }

    async function clearAll() {
      var db = await openDB();
      await clearAllDB(db);
      await render();
      if (typeof window.log === 'function') {
        window.log('Cleared IndexedDB device cache');
      }
    }

    async function render() {
      var output = document.getElementById('cache-lru-output');
      if (!output) return;
      try {
        var db = await openDB();
        var items = await getAllItems(db);
        items.sort(function(a, b) {
          return new Date(b.last_accessed_at || 0).getTime() - new Date(a.last_accessed_at || 0).getTime();
        });
        var totalBytes = items.reduce(function(sum, i) { return sum + (i.size_bytes || 0); }, 0);
        var display = items.map(function(i) {
          return { key: i.key, size_bytes: i.size_bytes, content_hash: i.content_hash, created_at: i.created_at, last_accessed_at: i.last_accessed_at };
        });
        output.textContent = JSON.stringify({
          limit_bytes: MAX_BYTES,
          limit_items: MAX_ITEMS,
          total_bytes: totalBytes,
          item_count: items.length,
          items: display
        }, null, 2);
      } catch (err) {
        output.textContent = 'Cache error: ' + err.message;
      }
    }

    window.clinicCache = {
      put: putItem,
      get: getItem,
      invalidateIfHashMismatch: invalidateIfHashMismatch,
      clearAll: clearAll,
      render: render,
      putFromForm: function() {
        var key = document.getElementById('cache-item-key').value;
        var size = document.getElementById('cache-item-size').value;
        var hash = document.getElementById('cache-item-hash').value;
        putItem(key, size, hash).then(render);
      }
    };

    render();
  })();
  </script>
</section>
`)
}
