import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const baselinePath = resolve(root, "svelte-check.baseline");
const command = resolve(root, "node_modules", ".bin", "svelte-check");
const result = spawnSync(command, ["--tsconfig", "./tsconfig.json", "--output", "machine"], {
  cwd: root,
  encoding: "utf8",
});

if (result.error) throw result.error;
process.stderr.write(result.stderr);

const output = result.stdout || "";
const lines = output.split("\n").filter(Boolean);
const normalizedLines = lines.map((line) => line.replace(/^[0-9]+ /, ""));
const diagnostics = normalizedLines
  .filter((line) => /^(?:ERROR|WARNING) /.test(line))
  .sort();
const summaries = normalizedLines.filter((line) => line.startsWith("COMPLETED "));
const unexpected = normalizedLines.filter(
  (line) =>
    !line.startsWith("START ") &&
    !line.startsWith("ERROR ") &&
    !line.startsWith("WARNING ") &&
    !line.startsWith("COMPLETED "),
);

if (summaries.length !== 1 || unexpected.length !== 0) {
  process.stdout.write(output);
  console.error("svelte-check returned incomplete or unrecognized machine output");
  process.exit(1);
}

const summary = summaries[0];
const match = summary.match(
  /^COMPLETED (\d+) FILES (\d+) ERRORS (\d+) WARNINGS (\d+) FILES_WITH_PROBLEMS$/,
);
if (!match) {
  process.stdout.write(output);
  console.error("svelte-check returned an unrecognized completion summary");
  process.exit(1);
}

const errorCount = Number(match[2]);
const warningCount = Number(match[3]);
const expectedStatus = errorCount === 0 ? 0 : 1;
if (result.status !== expectedStatus) {
  process.stdout.write(output);
  console.error(
    `svelte-check exited ${result.status}; expected ${expectedStatus} for its reported diagnostics`,
  );
  process.exit(1);
}
if (diagnostics.filter((line) => line.startsWith("ERROR ")).length !== errorCount ||
    diagnostics.filter((line) => line.startsWith("WARNING ")).length !== warningCount) {
  process.stdout.write(output);
  console.error("svelte-check summary does not match its emitted diagnostics");
  process.exit(1);
}

const normalized = [...diagnostics, summary].join("\n");

if (process.argv.includes("--update-baseline")) {
  writeFileSync(baselinePath, normalized + "\n");
  console.log("Updated " + baselinePath);
  process.exit(0);
}

const actual = normalized;
const expected = readFileSync(baselinePath, "utf8").trim();
if (actual !== expected) {
  process.stdout.write(output);
  console.error("svelte-check diagnostics differ from ui/svelte-check.baseline");
  console.error("Run 'yarn check --update-baseline' only when the diagnostic change is intentional and reviewed.");
  process.exit(1);
}

console.log(
  `svelte-check baseline matched (${errorCount} known errors, ${warningCount} known warnings); no diagnostic changes`,
);
