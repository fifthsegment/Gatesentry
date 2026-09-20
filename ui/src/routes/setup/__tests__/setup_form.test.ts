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

// Reasons that belong to one input are rendered as that field's own error text
// (Carbon's red requirement line) instead of a paragraph below the form, so the
// two assertions below have to be read together.
function hint() {
  return document.querySelector(".hint")?.textContent?.trim() ?? "";
}

function fieldError(text: string) {
  const message = screen.getByText(text);
  expect(message.className).toContain("bx--form-requirement");
  return message;
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
  await fill("Administrator username", "admin");
  await fill("Password", PASSWORD);
  await fill("Confirm password", PASSWORD);

  expect(button.disabled).toBe(false);
});

test("shows a mismatched confirmation as the confirm field's own error", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();

  await fill("Administrator username", "admin");
  await fill("Password", PASSWORD);
  await fill("Confirm password", PASSWORD_MISMATCH);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe("");
  fieldError("Passwords do not match.");
});

test("shows the UTF-8 byte limit as the password field's own error", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();

  await fill("Administrator username", "admin");
  await fill("Password", PASSWORD_SHORT);
  await fill("Confirm password", PASSWORD_SHORT);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe("");
  fieldError(
    "Password must contain between 12 and 72 UTF-8 bytes (currently 11).",
  );
});

test("counts UTF-8 bytes rather than JavaScript characters in the field error", async () => {
  mockStatus();
  render(Setup);
  const button = await setupButton();
  // U+1F512 is four UTF-8 bytes but two JavaScript UTF-16 code units.
  const tooLong = "\u{1F512}".repeat(19);

  await fill("Administrator username", "admin");
  await fill("Password", tooLong);
  await fill("Confirm password", tooLong);

  expect(button.disabled).toBe(true);
  expect(hint()).toBe("");
  fieldError(
    "Password must contain between 12 and 72 UTF-8 bytes (currently 76).",
  );
});
