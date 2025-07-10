import { readFileSync } from 'fs';
import { compile } from 'svelte/compiler';
import { test, expect } from 'vitest';
import path from 'path';

test('dns page contains resolver setting', () => {
  const file = path.resolve(__dirname, '../../routes/dns/dns.svelte');
  const source = readFileSync(file, 'utf-8');
  const { js } = compile(source, { generate: 'ssr' });
  expect(js.code).toMatch('dns_resolver');
});
