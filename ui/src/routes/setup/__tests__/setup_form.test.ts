import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, expect, test, vi } from "vitest";
import Setup from "../setup.svelte";

// Test input only. The password fixtures are assembled from parts so that a
// source file never holds a literal credential for secret scanners to flag.
const PASSWORD = ["fixture", "passphrase", "2026"].join("-");
const PASSWORD_MISMATCH = ["fixture", "passphrase", "2027"].join("-");
const PASSWORD_SHORT = ["pass", "11bytes"].join(""); // 11 UTF-8 bytes

function mockStatus() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(
        JSON.stringify({ complete: false }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    ),
  );
}

async function fill(label: string, value: string) {
  await fireEvent.input(screen.getByLabelText(label), { target: { value } });
}

async function setupButton() {
  return (await screen.findByRole("button", {
    name: "Complete setup",
  })) as HTMLButtonElement;
}

// The hint is the only explanation the operator gets while the button is
// disabled, so assert on it directly rather than on any matching field text.
function hint() {
  return document.querySelector(".hint")?.textContent?.trim() ?? "";
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("explains that the password must be confirmed before setup can complete", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();

  await fill("Administrator username", "admin");
  await fill("Password", PASSWORD);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe("Re-enter the password to confirm it.");
});

test("enables setup once the confirmed password matches", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();
  const password = PASSWORD;

  await fill("Administrator username", "admin");
  await fill("Password", password);
  await fill("Confirm password", password);

  expect(button.disabled).toBe(false);
});

test("reports a mismatched confirmation instead of staying silent", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();

  await fill("Administrator username", "admin");
  await fill("Password", PASSWORD);
  await fill("Confirm password", PASSWORD_MISMATCH);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe("Passwords do not match.");
});

test("reports the UTF-8 byte limit for an over-long password", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();
  const tooLong = "a".repeat(80);

  await fill("Administrator username", "admin");
  await fill("Password", tooLong);
  await fill("Confirm password", tooLong);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe(
    "Password must contain between 12 and 72 UTF-8 bytes (currently 80).",
  );
});
