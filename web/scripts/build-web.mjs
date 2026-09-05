import { mkdir, readFile, rm, writeFile, cp } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import * as esbuild from 'esbuild';

const execFileAsync = promisify(execFile);
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repo = resolve(root, '..');
const dist = resolve(root, 'dist');
const embedDist = resolve(repo, 'internal/server/web/dist');

async function read(rel) {
  return readFile(resolve(root, rel), 'utf8');
}

// Clean and recreate dist directories
await rm(dist, { recursive: true, force: true });
await mkdir(dist, { recursive: true });

// 1. Bundle Preact Application using esbuild
await esbuild.build({
  entryPoints: [resolve(root, 'src/main.jsx')],
  bundle: true,
  outfile: resolve(dist, 'app.js'),
  format: 'esm',
  jsx: 'automatic',
  jsxImportSource: 'preact',
  minify: true,
  target: ['es2020']
});

// 2. Compile Tailwind CSS
const tailwindBin = resolve(root, 'node_modules/.bin/tailwindcss');
try {
  await execFileAsync(tailwindBin, ['-i', resolve(root, 'src/styles/app.css'), '-o', resolve(dist, 'app.css'), '--minify'], { cwd: root });
} catch {
  await writeFile(resolve(dist, 'app.css'), await read('src/styles/app.css'), 'utf8');
}

// 3. Copy index.html, assets, and Web Workers
await cp(resolve(root, 'index.html'), resolve(dist, 'index.html'));
try {
  await cp(resolve(root, 'src/assets'), resolve(dist, 'assets'), { recursive: true });
  await cp(resolve(root, 'src/assets/logo.png'), resolve(dist, 'logo.png'));
} catch (_) {}
for (const worker of ['log-search.worker.js', 'timeline.worker.js', 'schema-layout.worker.js']) {
  try {
    await cp(resolve(root, 'src/workers', worker), resolve(dist, worker));
  } catch (_) {}
}

// 4. Sync to Go embedded directory
await rm(embedDist, { recursive: true, force: true });
await mkdir(embedDist, { recursive: true });
await cp(dist, embedDist, { recursive: true });

console.log(`Built DBVault Preact console into ${dist} and ${embedDist}`);
