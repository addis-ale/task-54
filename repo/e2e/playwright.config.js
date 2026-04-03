// @ts-check
const { defineConfig } = require("@playwright/test");
const chromiumPath =
  process.env.PW_CHROMIUM_EXECUTABLE ||
  "C:/Users/PC/AppData/Local/ms-playwright/chromium-1217/chrome-win64/chrome.exe";

module.exports = defineConfig({
  testDir: "./tests",
  timeout: 120000,
  use: {
    baseURL: "http://127.0.0.1:8080",
    trace: "on-first-retry",
    launchOptions: {
      executablePath: chromiumPath,
      args: ["--headless=new"],
    },
  },
  webServer: {
    command: "go run ./cmd/server",
    cwd: "..",
    url: "http://127.0.0.1:8080/api/v1/health",
    timeout: 120000,
    reuseExistingServer: true,
    env: {
      SESSION_COOKIE_SECURE: "false",
      BOOTSTRAP_ADMIN_USERNAME: "admin",
      BOOTSTRAP_ADMIN_PASSWORD: "AdminPassword1!",
      APP_DB_PATH: "./data/e2e.db",
      APP_REPORTS_SHARED_ROOT: "./data/shared_reports_e2e",
      APP_MASTER_KEY_B64: "ZTJlX3Rlc3Rfa2V5X2Zvcl9hZXMyNTZfMzJieXRlc1g=",
    },
  },
});
