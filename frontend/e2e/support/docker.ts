import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import type { Runtime } from './runtime'

const maxOutput = 16 << 10

function runDocker(root: string, environment: NodeJS.ProcessEnv, stdin: string | undefined, args: string[], secret: string) {
  return new Promise<string>((resolve, reject) => {
    const child = spawn('docker', args, { cwd: root, env: environment })
    let output = ''
    const append = (value: Buffer) => { output = (output + value.toString()).slice(-maxOutput) }
    child.stdout.on('data', append)
    child.stderr.on('data', append)
    child.once('error', reject)
    child.once('close', (status) => {
      const safeOutput = secret ? output.replaceAll(secret, '[REDACTED]') : output
      if (status === 0) resolve(safeOutput)
      else reject(new Error(`docker ${args.join(' ')} failed (${status ?? 'unknown'}): ${safeOutput}`))
    })
    if (stdin) child.stdin.end(stdin)
    else child.stdin.end()
  })
}

export async function pushFirstImage(runtime: Runtime, username: string, secret: string) {
  const registry = runtime.publicURL.replace('http://', '')
  const source = `grom-admin-e2e-${runtime.project}:source`
  const target = `${registry}/alpha/app:v1`
  const dockerConfig = await mkdtemp(join(tmpdir(), 'grom-admin-e2e-docker-'))
  const environment = { ...process.env, DOCKER_CONFIG: dockerConfig }
  await runDocker(runtime.root, environment, undefined, [
    'build', '--pull=false', '--tag', source,
    join(runtime.root, 'backend/tests/registrye2e/fixtures/variant-a'),
  ], '')
  await runDocker(runtime.root, environment, `${secret}\n`, ['login', '--username', username, '--password-stdin', registry], secret)
  await runDocker(runtime.root, environment, undefined, ['tag', source, target], secret)
  await runDocker(runtime.root, environment, undefined, ['push', target], secret)
  return async () => {
    await runDocker(runtime.root, environment, undefined, ['image', 'rm', '--force', target], secret).catch(() => undefined)
    await runDocker(runtime.root, environment, undefined, ['image', 'rm', '--force', source], secret).catch(() => undefined)
    await rm(dockerConfig, { recursive: true, force: true })
  }
}
