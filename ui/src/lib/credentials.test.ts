import { expect, test } from "vitest";
import {
  MAX_PASSWORD_BYTES,
  MIN_PASSWORD_BYTES,
  passwordHasValidByteLength,
  utf8ByteLength,
} from "./credentials";

test("password length uses the backend's UTF-8 byte contract", () => {
  expect(utf8ByteLength("a".repeat(MIN_PASSWORD_BYTES))).toBe(12);
  expect(passwordHasValidByteLength("a".repeat(MIN_PASSWORD_BYTES - 1))).toBe(false);
  expect(passwordHasValidByteLength("a".repeat(MIN_PASSWORD_BYTES))).toBe(true);
  expect(passwordHasValidByteLength("a".repeat(MAX_PASSWORD_BYTES))).toBe(true);
  expect(passwordHasValidByteLength("a".repeat(MAX_PASSWORD_BYTES + 1))).toBe(false);

  // U+1F512 is four UTF-8 bytes but two JavaScript UTF-16 code units. This is
  // the input shape that previously passed the form and failed at the API.
  expect(utf8ByteLength("🔒".repeat(18))).toBe(MAX_PASSWORD_BYTES);
  expect(passwordHasValidByteLength("🔒".repeat(18))).toBe(true);
  expect(passwordHasValidByteLength("🔒".repeat(19))).toBe(false);
});
