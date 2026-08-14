import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, resolve } from 'node:path';

const root = resolve(process.cwd(), 'dist');
const types = { '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css' };
createServer(async (req, res) => {
  const path = req.url === '/' ? '/index.html' : req.url.replace('/assets/', '/');
  try {
    const file = await readFile(resolve(root, '.' + path));
    res.writeHead(200, { 'content-type': types[extname(path)] || 'application/octet-stream' });
    res.end(file);
  } catch {
    const index = await readFile(resolve(root, 'index.html'));
    res.writeHead(200, { 'content-type': 'text/html' });
    res.end(index);
  }
}).listen(5173, () => console.log('DBVault web preview: http://127.0.0.1:5173'));
