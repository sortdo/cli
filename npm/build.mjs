// Builds the npm packages from GoReleaser's dist folder:
//
//   node npm/build.mjs <dist> <version> <out>
//
// One package per platform (@sortdo/cli-<os>-<cpu>) holds that platform's program, and the sortdo package depends on
// all of them as optional dependencies, so npm installs only the one for this system. No install scripts run.
import { chmodSync, copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const [dist, version, out] = process.argv.slice(2)
if (!dist || !/^\d+\.\d+\.\d+$/.test(version || '') || !out) {
    console.error('Usage: node npm/build.mjs <dist> <X.Y.Z> <out>')
    process.exit(1)
}
const here = new URL('.', import.meta.url).pathname
const common = {
    version,
    license: 'MIT',
    homepage: 'https://trysort.com/install',
    repository: { type: 'git', url: 'git+https://github.com/sortdo/cli.git' },
    bugs: { email: 'support@trysort.com' },
    publishConfig: { access: 'public', provenance: true },
}
const npmOs = { darwin: 'darwin', linux: 'linux', windows: 'win32' }
const npmCpu = { amd64: 'x64', arm64: 'arm64' }

rmSync(out, { recursive: true, force: true })
const binaries = JSON.parse(readFileSync(join(dist, 'artifacts.json'), 'utf8')).filter(artifact => artifact.type === 'Binary')
const optionalDependencies = {}
for (const artifact of binaries) {
    const os = npmOs[artifact.goos], cpu = npmCpu[artifact.goarch]
    if (!os || !cpu) throw new Error(`No npm platform for ${artifact.goos} ${artifact.goarch}`)
    const name = `@sortdo/cli-${os}-${cpu}`
    const folder = join(out, `cli-${os}-${cpu}`)
    const file = os === 'win32' ? 'sortdo.exe' : 'sortdo'
    mkdirSync(join(folder, 'bin'), { recursive: true })
    copyFileSync(artifact.path, join(folder, 'bin', file))
    chmodSync(join(folder, 'bin', file), 0o755)
    writeFileSync(join(folder, 'package.json'), JSON.stringify({
        name, description: `The sortdo program for ${os} ${cpu}. Install sortdo instead.`, ...common, os: [os], cpu: [cpu], files: ['bin'],
    }, null, 2) + '\n')
    writeFileSync(join(folder, 'README.md'), `# ${name}\n\nThe sortdo program for ${os} ${cpu}. Install [sortdo](https://www.npmjs.com/package/sortdo) instead.\n`)
    optionalDependencies[name] = version
}
if (Object.keys(optionalDependencies).length !== 6) throw new Error(`Expected 6 platforms, found ${Object.keys(optionalDependencies).length}`)

const main = join(out, 'sortdo')
mkdirSync(join(main, 'bin'), { recursive: true })
copyFileSync(join(here, 'sortdo.js'), join(main, 'bin', 'sortdo.js'))
chmodSync(join(main, 'bin', 'sortdo.js'), 0o755)
copyFileSync(join(here, '..', 'README.md'), join(main, 'README.md'))
copyFileSync(join(here, '..', 'LICENSE'), join(main, 'LICENSE'))
writeFileSync(join(main, 'package.json'), JSON.stringify({
    name: 'sortdo', description: 'Sort from the command line: search, read and write your team\'s knowledge, for people and coding agents.',
    keywords: ['sort', 'cli', 'agents', 'mcp', 'knowledge base'], ...common,
    bin: { sortdo: 'bin/sortdo.js' }, files: ['bin', 'README.md', 'LICENSE'], engines: { node: '>=18' }, optionalDependencies,
}, null, 2) + '\n')
console.log(`Built ${Object.keys(optionalDependencies).length + 1} npm packages for sortdo ${version} in ${out}`)
