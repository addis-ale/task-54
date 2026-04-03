const { test, expect } = require("@playwright/test");

async function login(page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(process.env.E2E_USERNAME || "admin");
  await page.getByLabel("Password").fill(process.env.E2E_PASSWORD || "AdminPassword1!");
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page).toHaveURL(/\/app$/, { timeout: 15000 });
  await expect(
    page.getByText("Server-Rendered Operations Console"),
  ).toBeVisible();
}

test("login/session and occupancy overview flow", async ({ page }) => {
  await login(page);

  await page.request.post("/api/v1/wards", {
    data: { name: "E2E Ward" },
    headers: { "Idempotency-Key": "e2e-ward-1" },
  });
  const wardRes = await page.request.get("/api/v1/wards");
  const wardBody = await wardRes.json();
  const wardID = wardBody.data.wards.find((w) => w.name === "E2E Ward").id;

  const patientRes = await page.request.post("/api/v1/patients", {
    data: { mrn: "E2E-MRN-1", name: "E2E Resident" },
    headers: { "Idempotency-Key": "e2e-patient-1" },
  });
  const patientBody = await patientRes.json();
  const patientID = patientBody.data.patient.id;

  const bedRes = await page.request.post("/api/v1/beds", {
    data: { ward_id: wardID, bed_code: "E2E-01" },
    headers: { "Idempotency-Key": "e2e-bed-1" },
  });
  const bedBody = await bedRes.json();
  const bedID = bedBody.data.bed.id;

  await page.request.post("/api/v1/admissions", {
    data: { patient_id: patientID, bed_id: bedID },
    headers: { "Idempotency-Key": "e2e-admission-1" },
  });

  await page.getByRole("button", { name: "Occupancy" }).click();
  await expect(page.locator("#panel")).toContainText("Occupancy Board");
  await expect(page.locator("#panel")).toContainText("E2E-01");

  await page.locator('form[action="/logout"] button').click();
  await expect(page).toHaveURL(/\/login$/);
});

test("exercise favorite and cache clear", async ({ page }) => {
  await login(page);

  const created = await page.request.post("/api/v1/exercises", {
    data: {
      title: "E2E Squat",
      description: "exercise for e2e",
      difficulty: "beginner",
      tags: ["legs"],
      equipment: ["none"],
      contraindications: [],
      body_regions: ["lower body"],
    },
    headers: { "Idempotency-Key": "e2e-exercise-1" },
  });
  const createdBody = await created.json();
  const exerciseID = createdBody.data.exercise.id;

  await page.request.post(`/api/v1/exercises/${exerciseID}/favorite`, {
    headers: { "Idempotency-Key": "e2e-favorite-toggle-1" },
  });

  const exercisePanel = await page.request.get("/ui/panels/exercises");
  const panelHTML = await exercisePanel.text();
  expect(panelHTML).toContain("Exercise Browse and Favorites");
  expect(panelHTML).toContain("Favorited");

  await page.goto("/app");
  await page.evaluate(() => {
    localStorage.setItem("clinic:lru:test:1", "cached");
    window.clearClinicDeviceCache();
  });
  const cacheCleared = await page.evaluate(() =>
    Object.keys(localStorage).every((k) => !k.startsWith("clinic:lru:")),
  );
  expect(cacheCleared).toBeTruthy();
});

test("scheduling conflict and publish, finance, reports", async ({ page }) => {
  test.setTimeout(120000);
  await login(page);

  const start = new Date(Date.now() + 4 * 60 * 60 * 1000);
  const end = new Date(start.getTime() + 3 * 60 * 60 * 1000);
  const startRFC = start.toISOString().replace(".000", "");
  const endRFC = end.toISOString().replace(".000", "");

  const templateCreate = await page.request.post("/api/v1/exam-templates", {
    data: {
      title: "E2E Template",
      subject: "Math",
      duration_minutes: 90,
      room_id: 801,
      proctor_id: 901,
      candidate_ids: [7001, 7002],
      window_label: "AM",
      window_start_at: startRFC,
      window_end_at: endRFC,
    },
    headers: { "Idempotency-Key": "e2e-template-create" },
  });
  expect(templateCreate.ok()).toBeTruthy();

  const templatesRes = await page.request.get("/api/v1/exam-templates");
  const templatesBody = await templatesRes.json();
  const tpl = templatesBody.data.exam_templates[0];
  const templateID = tpl.id;
  const windowID = tpl.windows[0].id;

  await page.request.post("/api/v1/exam-schedules", {
    data: {
      exam_id: "conflict-existing",
      room_id: tpl.room_id,
      proctor_id: tpl.proctor_id,
      candidate_ids: [7001],
      start_at: startRFC,
      end_at: new Date(start.getTime() + 90 * 60 * 1000)
        .toISOString()
        .replace(".000", ""),
    },
    headers: { "Idempotency-Key": "e2e-existing-schedule" },
  });

  const draftCreate = await page.request.post(
    "/api/v1/exam-session-drafts/generate",
    {
      data: { template_id: templateID, window_id: windowID },
      headers: { "Idempotency-Key": "e2e-draft-create" },
    },
  );
  expect(draftCreate.ok()).toBeTruthy();

  const draftsRes = await page.request.get("/api/v1/exam-session-drafts");
  const draftsBody = await draftsRes.json();
  const draft = draftsBody.data.exam_session_drafts[0];
  expect(draft.conflicts.length).toBeGreaterThan(0);

  const adjustedStart = new Date(end.getTime() + 30 * 60 * 1000);
  const adjustedEnd = new Date(adjustedStart.getTime() + 90 * 60 * 1000);
  await page.request.post(`/api/v1/exam-session-drafts/${draft.id}/adjust`, {
    data: {
      start_at: adjustedStart.toISOString().replace(".000", ""),
      end_at: adjustedEnd.toISOString().replace(".000", ""),
    },
    headers: { "Idempotency-Key": "e2e-draft-adjust" },
  });
  await page.request.post(`/api/v1/exam-session-drafts/${draft.id}/publish`, {
    headers: { "Idempotency-Key": "e2e-draft-publish" },
  });

  const paymentCreate = await page.request.post("/api/v1/payments", {
    data: {
      method: "cash",
      gateway: "cash_local",
      amount_cents: 20000,
      currency: "USD",
      shift_id: "shift-0700",
    },
    headers: { "Idempotency-Key": "e2e-payment-create" },
  });
  expect(paymentCreate.ok()).toBeTruthy();

  const paymentsRes = await page.request.get("/api/v1/payments");
  const paymentsBody = await paymentsRes.json();
  const paymentID = paymentsBody.data.payments[0].id;
  const refund = await page.request.post(
    `/api/v1/payments/${paymentID}/refunds`,
    {
      data: { amount_cents: 5000, reason: "e2e_refund" },
      headers: { "Idempotency-Key": "e2e-payment-refund" },
    },
  );
  expect(refund.ok()).toBeTruthy();

  const settlement = await page.request.post("/api/v1/settlements/run", {
    data: { shift_id: "shift-0700", actual_total_cents: 15000 },
    headers: { "Idempotency-Key": "e2e-settlement" },
  });
  expect(settlement.ok()).toBeTruthy();

  const financeExport = await page.request.get(
    "/api/v1/reports/finance/export?format=csv",
  );
  expect(financeExport.ok()).toBeTruthy();

  const auditExport = await page.request.get(
    "/api/v1/reports/audit/export?format=csv",
  );
  expect(auditExport.ok()).toBeTruthy();

  const scheduleCreate = await page.request.post("/api/v1/reports/schedules", {
    data: {
      report_type: "audit",
      format: "csv",
      shared_folder_path: "./data/shared_reports_e2e",
      interval_minutes: 5,
      first_run_at: new Date(Date.now() - 60000)
        .toISOString()
        .replace(".000", ""),
    },
    headers: { "Idempotency-Key": "e2e-report-schedule" },
  });
  expect(scheduleCreate.ok()).toBeTruthy();
  const runSchedules = await page.request.post(
    "/api/v1/reports/schedules/run-now",
    {
      headers: { "Idempotency-Key": "e2e-report-schedule-run" },
    },
  );
  expect(runSchedules.ok()).toBeTruthy();

  const configVersion = await page.request.post("/api/v1/config/versions", {
    data: {
      config_key: "e2e",
      payload_json: '{"foo":"bar"}',
    },
    headers: { "Idempotency-Key": "e2e-config-create" },
  });
  expect(configVersion.ok()).toBeTruthy();
  const versionBody = await configVersion.json();
  const versionID = versionBody.data.config_version.id;
  const rollback = await page.request.post(
    `/api/v1/config/versions/${versionID}/rollback`,
    {
      headers: { "Idempotency-Key": "e2e-config-rollback" },
    },
  );
  expect(rollback.ok()).toBeTruthy();

  const reportsPanel = await page.request.get("/ui/panels/reports");
  const reportsHTML = await reportsPanel.text();
  expect(reportsHTML).toContain("Audit Search");
  expect(reportsHTML).toContain("Config Versioning and Rollback");
});
