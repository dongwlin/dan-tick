import { createHash } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { copyFile, mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

const platform = process.argv[2]
const version = process.env.DAN_TICK_VERSION
if (!version || !/^[a-zA-Z0-9.-]+$/.test(version)) {
  throw new Error('DAN_TICK_VERSION must be a safe artifact version')
}
const suffix = `${version}-${platform}-amd64`
const files = {
  linux: [['dan-tick-x86_64.AppImage', `dan-tick-${suffix}.AppImage`]],
  windows: [
    ['dan-tick.exe', `dan-tick-${suffix}.exe`],
    ['dan-tick-amd64-installer.exe', `dan-tick-${suffix}-installer.exe`],
  ],
}[platform]
if (!files) throw new Error('Expected linux or windows')

const output = path.join('bin', 'artifacts')
await mkdir(output, { recursive: true })
for (const [source, name] of files) {
  const destination = path.join(output, name)
  await copyFile(path.join('bin', source), destination)
  const hash = createHash('sha256')
  for await (const chunk of createReadStream(destination)) hash.update(chunk)
  // Each artifact ships exactly one sidecar checksum named after it, in the
  // format `sha256sum --check` reads, so no aggregate manifest is needed.
  await writeFile(`${destination}.sha256`, `${hash.digest('hex')}  ${name}\n`)
}
