import assert from "node:assert/strict";
import { chromium } from "@playwright/test";

const browser = await chromium.launch({ channel: "chrome", headless: true });
try {
  const page = await browser.newPage();
  const errors = [];
  const writes = [];
  let fail = false;
  page.on("pageerror", error => errors.push(error.message));
  await page.route("**/test/save", route => {
    writes.push(route.request().postDataJSON());
    return route.fulfill({ status: fail ? 500 : 200, body: "" });
  });
  await page.goto(`${process.env.TEST_URL || "http://127.0.0.1:5180"}/tests/bitrate-harness.html`);
  const input = page.getByRole("spinbutton");
  await input.waitFor();
  assert.equal(await input.inputValue(), "17500");
  assert.equal(await page.getByRole("combobox").inputValue(), "17500");
  await page.getByRole("slider").focus();
  await page.keyboard.press("ArrowRight");
  assert.equal(await input.inputValue(), "17600");
  assert.equal(writes.length, 0, "editing must not restart the encoder");
  await page.getByRole("button").click();
  await page.waitForFunction(() => localStorage.getItem("test-factor") === String(17600 / 12000));
  assert.equal(writes[0].factor, 17600 / 12000);
  await page.reload();
  await input.waitFor();
  assert.equal(await input.inputValue(), "17600");
  await page.getByRole("combobox").selectOption("60000");
  assert.equal(await input.inputValue(), "60000");
  fail = true;
  await page.getByRole("button").click();
  await page.waitForFunction(
    () => document.querySelector('input[type="number"]').value === "17600",
  );
  await input.fill("60001");
  assert(await page.getByRole("button").isDisabled());
  await input.fill("");
  assert(await page.getByRole("button").isDisabled());
  assert.equal(writes.length, 2);
  assert.deepEqual(errors, []);
  console.log(
    "PASS: slider/preset/input sync, explicit save, reload, failed save rollback, invalid bounds.",
  );
} finally {
  await browser.close();
}
