// Regenerates src/routeTree.gen.ts from src/routes using tsr.config.json.
// Usage: node ./scripts/generate-routes.mjs [--root <directory containing tsr.config.json>]
import { resolve } from 'node:path'
import { parseArgs } from 'node:util'
import { Generator, getConfig } from '@tanstack/router-generator'

const { values } = parseArgs({ options: { root: { type: 'string' } } })
const root = resolve(values.root ?? process.cwd())
// Keep the generator's temp files beside the output so its final rename never crosses drives.
const config = getConfig({ tmpDir: resolve(root, '.tanstack/tmp') }, root)

await new Generator({ config, root }).run()
