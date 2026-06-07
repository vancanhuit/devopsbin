// Vendors the Swagger UI and Redoc consoles into the Vite build output so the
// Go binary can embed and serve them entirely same-origin (no CDN), keeping the
// container image self-contained and the documentation CSP at script-src
// 'self'.
//
// Runs after `vite build` (see package.json "build"). It copies the committed
// HTML/initializer from web/docs/ together with the prebuilt bundles shipped by
// the swagger-ui-dist and redoc packages into dist/swagger/ and dist/redoc/,
// which //go:embed all:dist then picks up.
import { cp, mkdir } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'

const root = resolve(import.meta.dirname, '..')
const dist = resolve(root, 'dist')
const nodeModules = resolve(root, 'node_modules')

// Each entry is [source, destination] resolved against `root`.
const copies = [
    // First-party page shells and the Swagger UI bootstrap.
    [resolve(root, 'docs/swagger'), resolve(dist, 'swagger')],
    [resolve(root, 'docs/redoc'), resolve(dist, 'redoc')],
    // Vendored prebuilt bundles.
    [
        resolve(nodeModules, 'swagger-ui-dist/swagger-ui.css'),
        resolve(dist, 'swagger/swagger-ui.css'),
    ],
    [
        resolve(nodeModules, 'swagger-ui-dist/swagger-ui-bundle.js'),
        resolve(dist, 'swagger/swagger-ui-bundle.js'),
    ],
    [
        resolve(nodeModules, 'redoc/bundles/redoc.standalone.js'),
        resolve(dist, 'redoc/redoc.standalone.js'),
    ],
]

for (const [src, destPath] of copies) {
    await mkdir(dirname(destPath), { recursive: true })
    await cp(src, destPath, { recursive: true })
}

console.log('vendored Swagger UI and Redoc into dist/')
