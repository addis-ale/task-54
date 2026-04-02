# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: careops.spec.js >> login/session and occupancy overview flow
- Location: tests\careops.spec.js:14:1

# Error details

```
Error: expect(page).toHaveURL(expected) failed

Expected pattern: /\/app$/
Received string:  "http://127.0.0.1:8080/login"
Timeout: 15000ms

Call log:
  - Expect "toHaveURL" with timeout 15000ms
    18 × unexpected value "http://127.0.0.1:8080/login"

```

# Page snapshot

```yaml
- generic [ref=e2]: invalid credentials
```

# Test source

```ts
  1   | const { test, expect } = require("@playwright/test");
  2   | 
  3   | async function login(page) {
  4   |   await page.goto("/login");
  5   |   await page.getByLabel("Username").fill("admin");
  6   |   await page.getByLabel("Password").fill("AdminPassword1!");
  7   |   await page.getByRole("button", { name: "Sign In" }).click();
> 8   |   await expect(page).toHaveURL(/\/app$/, { timeout: 15000 });
      |                      ^ Error: expect(page).toHaveURL(expected) failed
  9   |   await expect(
  10  |     page.getByText("Server-Rendered Operations Console"),
  11  |   ).toBeVisible();
  12  | }
  13  | 
  14  | test("login/session and occupancy overview flow", async ({ page }) => {
  15  |   await login(page);
  16  | 
  17  |   await page.request.post("/api/v1/wards", {
  18  |     data: { name: "E2E Ward" },
  19  |     headers: { "Idempotency-Key": "e2e-ward-1" },
  20  |   });
  21  |   const wardRes = await page.request.get("/api/v1/wards");
  22  |   const wardBody = await wardRes.json();
  23  |   const wardID = wardBody.data.wards.find((w) => w.name === "E2E Ward").id;
  24  | 
  25  |   const patientRes = await page.request.post("/api/v1/patients", {
  26  |     data: { mrn: "E2E-MRN-1", name: "E2E Resident" },
  27  |     headers: { "Idempotency-Key": "e2e-patient-1" },
  28  |   });
  29  |   const patientBody = await patientRes.json();
  30  |   const patientID = patientBody.data.patient.id;
  31  | 
  32  |   const bedRes = await page.request.post("/api/v1/beds", {
  33  |     data: { ward_id: wardID, bed_code: "E2E-01" },
  34  |     headers: { "Idempotency-Key": "e2e-bed-1" },
  35  |   });
  36  |   const bedBody = await bedRes.json();
  37  |   const bedID = bedBody.data.bed.id;
  38  | 
  39  |   await page.request.post("/api/v1/admissions", {
  40  |     data: { patient_id: patientID, bed_id: bedID },
  41  |     headers: { "Idempotency-Key": "e2e-admission-1" },
  42  |   });
  43  | 
  44  |   await page.getByRole("button", { name: "Occupancy" }).click();
  45  |   await expect(page.locator("#panel")).toContainText("Occupancy Board");
  46  |   await expect(page.locator("#panel")).toContainText("E2E-01");
  47  | 
  48  |   await page.locator('form[action="/logout"] button').click();
  49  |   await expect(page).toHaveURL(/\/login$/);
  50  | });
  51  | 
  52  | test("exercise favorite and cache clear", async ({ page }) => {
  53  |   await login(page);
  54  | 
  55  |   const created = await page.request.post("/api/v1/exercises", {
  56  |     data: {
  57  |       title: "E2E Squat",
  58  |       description: "exercise for e2e",
  59  |       difficulty: "beginner",
  60  |       tags: ["legs"],
  61  |       equipment: ["none"],
  62  |       contraindications: [],
  63  |       body_regions: ["lower body"],
  64  |     },
  65  |     headers: { "Idempotency-Key": "e2e-exercise-1" },
  66  |   });
  67  |   const createdBody = await created.json();
  68  |   const exerciseID = createdBody.data.exercise.id;
  69  | 
  70  |   await page.request.post(`/api/v1/exercises/${exerciseID}/favorite`, {
  71  |     headers: { "Idempotency-Key": "e2e-favorite-toggle-1" },
  72  |   });
  73  | 
  74  |   const exercisePanel = await page.request.get("/ui/panels/exercises");
  75  |   const panelHTML = await exercisePanel.text();
  76  |   expect(panelHTML).toContain("Exercise Browse and Favorites");
  77  |   expect(panelHTML).toContain("Favorited");
  78  | 
  79  |   await page.goto("/app");
  80  |   await page.evaluate(() => {
  81  |     localStorage.setItem("clinic:lru:test:1", "cached");
  82  |     window.clearClinicDeviceCache();
  83  |   });
  84  |   const cacheCleared = await page.evaluate(() =>
  85  |     Object.keys(localStorage).every((k) => !k.startsWith("clinic:lru:")),
  86  |   );
  87  |   expect(cacheCleared).toBeTruthy();
  88  | });
  89  | 
  90  | test("scheduling conflict and publish, finance, reports", async ({ page }) => {
  91  |   test.setTimeout(120000);
  92  |   await login(page);
  93  | 
  94  |   const start = new Date(Date.now() + 4 * 60 * 60 * 1000);
  95  |   const end = new Date(start.getTime() + 3 * 60 * 60 * 1000);
  96  |   const startRFC = start.toISOString().replace(".000", "");
  97  |   const endRFC = end.toISOString().replace(".000", "");
  98  | 
  99  |   const templateCreate = await page.request.post("/api/v1/exam-templates", {
  100 |     data: {
  101 |       title: "E2E Template",
  102 |       subject: "Math",
  103 |       duration_minutes: 90,
  104 |       room_id: 801,
  105 |       proctor_id: 901,
  106 |       candidate_ids: [7001, 7002],
  107 |       window_label: "AM",
  108 |       window_start_at: startRFC,
```