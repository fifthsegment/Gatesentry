import { readFileSync } from "fs";
import { compile } from "svelte/compiler";
import { test, expect } from "vitest";
import path from "path";

test("AI page exposes provider radio, API keys, and Alpha badge", () => {
  const file = path.resolve(__dirname, "../../routes/ai/ai.svelte");
  const source = readFileSync(file, "utf-8");
  const { js } = compile(source, { generate: "dom" });
  expect(js.code).toMatch("ai_image_filtering_mode");
  expect(js.code).toMatch("ai_grok_api_key");
  expect(js.code).toMatch("ai_openai_api_key");
  expect(js.code).toMatch("Enable Grok");
  expect(js.code).toMatch("Enable ChatGPT");
  expect(js.code).toMatch("Enable Local LLM");
  expect(js.code).toMatch("ai_local_llm_url");
  expect(js.code).toMatch("ai_grok_model");
  expect(js.code).toMatch("ai_openai_model");
  expect(js.code).toMatch("ai_local_llm_model");
  expect(js.code).toMatch("/ai/status");
  expect(js.code).toMatch("providerProbe");
  expect(js.code).toMatch("Reachable");
  expect(js.code).toMatch("Alpha");
});
