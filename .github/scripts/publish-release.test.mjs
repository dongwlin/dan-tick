import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, readdir, writeFile, chmod, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import { test } from 'node:test'

const stage = fileURLToPath(new URL('./stage-artifacts.mjs', import.meta.url))
const publish = fileURLToPath(new URL('./publish-release.sh', import.meta.url))
const version = 'v1.2.3'

async function fixture(t) {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'dan-tick-release-test-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  await mkdir(path.join(directory, 'bin'))
  for (const name of ['dan-tick-x86_64.AppImage', 'dan-tick.exe', 'dan-tick-amd64-installer.exe']) {
    await writeFile(path.join(directory, 'bin', name), `fixture binary: ${name}\n`)
  }
  for (const platform of ['linux', 'windows']) {
    const result = spawnSync(process.execPath, [stage, platform], {
      cwd: directory,
      env: { ...process.env, DAN_TICK_VERSION: version },
      encoding: 'utf8',
    })
    assert.equal(result.status, 0, result.stderr)
  }
  await mkdir(path.join(directory, 'mock-bin'))
  const mock = path.join(directory, 'mock-bin', 'gh')
  await writeFile(mock, `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MOCK_GH_LOG"
case "$2" in
  view)
    case "$MOCK_RELEASE_STATE" in
      published) echo false ;;
      draft) echo true ;;
      new) exit 1 ;;
    esac ;;
  upload) if [[ "$MOCK_FAIL_UPLOAD" == 1 ]]; then exit 1; fi ;;
esac
`)
  await chmod(mock, 0o755)
  const log = path.join(directory, 'gh.log')
  return {
    directory,
    assets: path.join(directory, 'bin', 'artifacts'),
    run(state = 'new', failUpload = '0') {
      // The production script only accesses the fake CLI in these tests.
      return spawnSync('bash', [publish], {
        cwd: directory,
        env: {
          ...process.env,
          RELEASE_TAG: version,
          PATH: `${path.join(directory, 'mock-bin')}${path.delimiter}${process.env.PATH}`,
          MOCK_GH_LOG: log,
          MOCK_RELEASE_STATE: state,
          MOCK_FAIL_UPLOAD: failUpload,
        },
        encoding: 'utf8',
      })
    },
    async prepare() {
      const { rename } = await import('node:fs/promises')
      await rename(path.join(directory, 'bin', 'artifacts'), path.join(directory, 'release-assets'))
    },
    async commands() {
      return (await readFile(log, 'utf8')).trim().split('\n')
    },
  }
}

test('staged checksums allow a complete draft to be uploaded before publication', async (t) => {
  const f = await fixture(t)
  await f.prepare()
  const result = f.run()
  assert.equal(result.status, 0, result.stderr)
  const commands = await f.commands()
  assert.equal(commands.length, 4)
  assert.match(commands[1], /^release create v1\.2\.3 --verify-tag --draft /)
  assert.match(commands[2], /^release upload v1\.2\.3 /)
  assert.match(commands[2], /windows-amd64-installer\.exe/)
  for (const name of [
    'linux-amd64.AppImage.sha256',
    'windows-amd64.exe.sha256',
    'windows-amd64-installer.exe.sha256',
  ]) {
    assert.match(commands[2], new RegExp(name.replaceAll('.', '\\.')))
  }
  assert.doesNotMatch(commands[2], /SHA256SUMS/)
  assert.equal(commands[3], 'release edit v1.2.3 --draft=false')
})

test('each artifact is staged with exactly one sidecar checksum named after it', async (t) => {
  const f = await fixture(t)
  const artifacts = [
    `dan-tick-${version}-linux-amd64.AppImage`,
    `dan-tick-${version}-windows-amd64.exe`,
    `dan-tick-${version}-windows-amd64-installer.exe`,
  ]
  const staged = (await readdir(f.assets)).sort()
  assert.deepEqual(staged, [...artifacts, ...artifacts.map((name) => `${name}.sha256`)].sort())
  for (const name of artifacts) {
    const sidecar = await readFile(path.join(f.assets, `${name}.sha256`), 'utf8')
    // The sidecar names the artifact it covers, so sha256sum finds it by itself.
    assert.match(sidecar, /^[0-9a-f]{64} {2}\S+\n$/)
    assert.equal(sidecar.split('\n')[0].slice(66), name)
    const check = spawnSync('sha256sum', ['--check', `${name}.sha256`], {
      cwd: f.assets,
      encoding: 'utf8',
    })
    assert.equal(check.status, 0, check.stderr)
  }
})

test('an upload failure leaves the draft unpublished', async (t) => {
  const f = await fixture(t)
  await f.prepare()
  assert.notEqual(f.run('new', '1').status, 0)
  assert.equal((await f.commands()).some((command) => command.includes('release edit')), false)
})

test('retrying an existing draft finishes it without creating another release', async (t) => {
  const f = await fixture(t)
  await f.prepare()
  assert.equal(f.run('draft').status, 0)
  const commands = await f.commands()
  assert.equal(commands.length, 3)
  assert.equal(commands.some((command) => command.includes('release create')), false)
  assert.equal(commands.at(-1), 'release edit v1.2.3 --draft=false')
})

test('published release assets are not overwritten', async (t) => {
  const f = await fixture(t)
  await f.prepare()
  assert.equal(f.run('published').status, 0)
  assert.deepEqual(await f.commands(), ['release view v1.2.3 --json isDraft --jq .isDraft'])
})

test('a corrupted artifact stops the process before contacting GitHub', async (t) => {
  const f = await fixture(t)
  await writeFile(path.join(f.assets, `dan-tick-${version}-windows-amd64.exe`), 'corrupted data')
  await f.prepare()
  assert.notEqual(f.run().status, 0)
  await assert.rejects(f.commands(), { code: 'ENOENT' })
})

test('a missing installer stops the process before contacting GitHub', async (t) => {
  const f = await fixture(t)
  await rm(path.join(f.assets, `dan-tick-${version}-windows-amd64-installer.exe`))
  await f.prepare()
  assert.notEqual(f.run().status, 0)
  await assert.rejects(f.commands(), { code: 'ENOENT' })
})

test('a missing sidecar checksum stops the process before contacting GitHub', async (t) => {
  const f = await fixture(t)
  await rm(path.join(f.assets, `dan-tick-${version}-linux-amd64.AppImage.sha256`))
  await f.prepare()
  assert.notEqual(f.run().status, 0)
  await assert.rejects(f.commands(), { code: 'ENOENT' })
})
