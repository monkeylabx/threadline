// Conservative CI routing. Unknown paths and manual runs retain full builds.
import { execFileSync } from "node:child_process";
import { appendFileSync, realpathSync, existsSync } from "node:fs";
import { dirname, resolve, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

export function narrative(path) {
  return /^(README|CONTRIBUTING|CONTEXT|AGENTS)\.md$/.test(path) ||
    /^docs\/(acceptance|adr|agents|architecture|build|contracts|design|development|integration|quality|research|security)\/(?:[^/]+\/)*[^/]+\.md$/.test(path) ||
    /^docs\/(delivery-plan|agent-workstreams)\.md$/.test(path);
}

function git(root, args) {
  return execFileSync("git", args, { cwd: root, stdio: ["ignore", "pipe", "pipe"], encoding: "utf8", maxBuffer: 16 * 1024 * 1024 });
}

export function changedPaths(root, base, head) {
  for (const ref of [base, head]) {
    if (!/^[a-f0-9]{40}$/.test(ref ?? "")) throw new Error("Invalid comparison commit");
    git(root, ["cat-file", "-e", `${ref}^{commit}`]);
  }
  return git(root, ["diff", "--no-renames", "--name-only", "-z", base, head, "--"])
    .split("\0").filter(Boolean);
}

export function classify(root, event, base, head) {
  if (event !== "pull_request") return { docsOnly: false, paths: [] };
  const paths = changedPaths(root, base, head);
  const docsOnly = paths.length > 0 && paths.every((path) => {
    if (!narrative(path)) return false;
    return [base, head].every((ref) => {
      const entry = git(root, ["ls-tree", ref, "--", path]);
      return entry === "" || entry.startsWith("100644 blob ");
    });
  });
  return { docsOnly, paths };
}

function destination(text, start) {
  if (text[start] === "<") {
    const end = text.indexOf(">", start + 1);
    return end < 0 ? null : text.slice(start + 1, end);
  }
  let depth = 0;
  let value = "";
  for (let index = start; index < text.length; index++) {
    const character = text[index];
    if (character === "\\" && index + 1 < text.length) {
      value += text[++index];
    } else if (character === "(") {
      depth++;
      value += character;
    } else if (character === ")") {
      if (depth === 0) return value;
      depth--;
      value += character;
    } else if (/\s/.test(character)) {
      return depth === 0 ? value : null;
    } else {
      value += character;
    }
  }
  return null;
}

export function localLinks(markdown) {
  // Check inline/image destinations and reference definitions; not anchors or HTML.
  const prose = markdown.replace(/^[ \t]*(`{3,}|~{3,})[^\n]*\n[\s\S]*?^[ \t]*\1[^\n]*$/gm, "")
    .replace(/`[^`\n]*`/g, "");
  const links = [...prose.matchAll(/\]\(\s*/g)]
    .map((match) => destination(prose, match.index + match[0].length)).filter(Boolean);
  for (const match of prose.matchAll(/^\s{0,3}\[[^\]]+\]:\s*(<[^>]+>|\S+)/gm)) links.push(match[1]);
  return links.map((link) => link.replace(/^<|>$/g, ""))
    .filter((link) => !/^(?:[a-z][a-z0-9+.-]*:|\/\/|#)/i.test(link))
    .map((link) => decodeURIComponent(link.split(/[?#]/)[0])).filter(Boolean);
}

export function checkDocs(root, base, head, paths) {
  const docs = paths.filter(narrative);
  if (docs.length === 0) return;
  git(root, ["diff", "--check", base, head, "--", ...docs]);
  const canonicalRoot = realpathSync(root);
  for (const path of docs) {
    const entry = git(root, ["ls-tree", head, "--", path]);
    if (!entry.startsWith("100644 blob ")) continue; // Deleted/non-document paths use full CI.
    const source = git(root, ["show", `${head}:${path}`]);
    for (const link of localLinks(source)) {
      const target = link.startsWith("/") ? resolve(canonicalRoot, `.${link}`) : resolve(canonicalRoot, dirname(path), link);
      const inside = (value) => {
        const rel = relative(canonicalRoot, value);
        return rel !== ".." && !rel.startsWith(`..${sep}`) && !resolve(value).startsWith(`${sep}${sep}`);
      };
      if (!inside(target) || !existsSync(target) || !inside(realpathSync(target))) {
        throw new Error(`Missing or escaping local Markdown target in ${JSON.stringify(path)}: ${JSON.stringify(link)}`);
      }
    }
  }
}

function main() {
  const root = process.cwd();
  const event = process.env.GITHUB_EVENT_NAME;
  const base = process.env.PR_BASE_SHA;
  const head = process.env.GITHUB_SHA;
  const result = classify(root, event, base, head);
  if (event === "pull_request") {
    if (git(root, ["rev-parse", "HEAD"]).trim() !== head) throw new Error("Checkout is not the tested commit");
    checkDocs(root, base, head, result.paths);
  }
  const mode = result.docsOnly ? "Documentation checks; platform builds NOT RUN" : "Full platform build";
  console.log(mode);
  // Publish only after all validation succeeds. Failed checks never emit a skip decision.
  appendFileSync(process.env.GITHUB_OUTPUT, `docs_only=${result.docsOnly}\n`);
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${mode}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
