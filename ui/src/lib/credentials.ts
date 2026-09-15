export const MIN_PASSWORD_BYTES = 12;
export const MAX_PASSWORD_BYTES = 72;

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function passwordHasValidByteLength(value: string): boolean {
  const length = utf8ByteLength(value);
  return length >= MIN_PASSWORD_BYTES && length <= MAX_PASSWORD_BYTES;
}
