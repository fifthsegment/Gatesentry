import { test, expect } from "vitest";
import source from "../../routes/ai/ai.svelte?raw";

test("AI page exposes provider radio, selected configuration, and Alpha badge", () => {
  expect(source).toMatch("ai_image_filtering_mode");
  expect(source).toMatch("ai_grok_api_key");
  expect(source).toMatch("ai_openai_api_key");
  expect(source).toMatch("Enable Grok");
  expect(source).toMatch("Enable ChatGPT");
  expect(source).toMatch("Enable Local LLM");
  expect(source).toMatch("ai_local_llm_url");
  expect(source).toMatch("ai_grok_model");
  expect(source).toMatch("ai_openai_model");
  expect(source).toMatch("ai_local_llm_model");
  expect(source).toMatch("/ai/status");
  expect(source).toMatch("providerProbe");
  expect(source).toMatch("Reachable");
  expect(source).toMatch("Alpha");
  expect(source).toMatch("AI image filtering");
  expect(source).toContain("<PageShell");
  expect(source).toMatch("Selected provider status");
  expect(source).toMatch("Legacy local scanner");
  expect(source).toMatch("Advanced");
  expect(source).toContain('{#if filterMode === "grok"}');
  expect(source).toContain('{:else if filterMode === "chatgpt"}');
  expect(source).toContain('{:else if filterMode === "local"}');
  expect(source).toContain("<AccordionItem");
  expect(source).toContain('orientation="vertical"');
});
