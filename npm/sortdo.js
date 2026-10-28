#!/usr/bin/env node
// The sortdo npm package: starts the sortdo program for this system, which npm installed as an optional dependency
// (@sortdo/cli-<os>-<cpu>). The program is the same one the install script and Homebrew install.
'use strict'
const { spawnSync } = require('node:child_process')

const name = `@sortdo/cli-${process.platform}-${process.arch}`
let program
try {
    program = require.resolve(`${name}/bin/sortdo${process.platform === 'win32' ? '.exe' : ''}`)
} catch {
    console.error(`sortdo: ${name} is not installed. sortdo supports macOS, Linux and Windows on x64 and arm64.`)
    console.error('Reinstall without --omit=optional (or --no-optional), or use: curl -fsSL https://sort.do/install | bash')
    process.exit(1)
}
const result = spawnSync(program, process.argv.slice(2), { stdio: 'inherit', windowsHide: false })
if (result.error) {
    console.error(`sortdo: could not start ${program}: ${result.error.message}`)
    process.exit(1)
}
if (result.signal) process.kill(process.pid, result.signal)
process.exit(result.status ?? 1)
