import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import type { Runtime } from './runtime'

const maxOutput = 16 << 10
const commandTimeout = 6 * 60_000

function runDocker(root: string, environment: NodeJS.ProcessEnv, stdin: string | undefined, args: string[], secret: string) {
  return new Promise<string>((resolve, reject) => {
    const child = spawn('docker', args, { cwd: root, env: environment })
    let settled = false
    let output = ''
    const redact = (value: string) => secret ? value.replaceAll(secret, '[REDACTED]') : value
    const settle = (result: () => void) => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      result()
    }
    const timer = setTimeout(() => {
      child.kill('SIGKILL')
      settle(() => reject(new Error(`docker ${args.join(' ')} timed out after six minutes`)))
    }, commandTimeout)
    const append = (value: Buffer) => { output = redact(output + value.toString()).slice(-maxOutput) }
    child.stdout.on('data', append)
    child.stderr.on('data', append)
    child.once('error', (error) => settle(() => reject(error)))
    child.once('close', (status) => {
      const safeOutput = redact(output)
      if (status === 0) settle(() => resolve(safeOutput))
      else settle(() => reject(new Error(`docker ${args.join(' ')} failed (${status ?? 'unknown'}): ${safeOutput}`)))
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
  const cleanup = async () => {
    await runDocker(runtime.root, environment, undefined, ['image', 'rm', '--force', target], secret).catch(() => undefined)
    await runDocker(runtime.root, environment, undefined, ['image', 'rm', '--force', source], secret).catch(() => undefined)
    await rm(dockerConfig, { recursive: true, force: true })
  }
  try {
    await runDocker(runtime.root, environment, undefined, [
      'build', '--pull=false', '--tag', source,
      join(runtime.root, 'backend/tests/registrye2e/fixtures/variant-a'),
    ], '')
    await runDocker(runtime.root, environment, `${secret}\n`, ['login', '--username', username, '--password-stdin', registry], secret)
    await runDocker(runtime.root, environment, undefined, ['tag', source, target], secret)
    await runDocker(runtime.root, environment, undefined, ['push', target], secret)
    const digestReference = await runDocker(runtime.root, environment, undefined, ['image', 'inspect', '--format', '{{index .RepoDigests 0}}', target], secret)
    const digest = digestReference.trim().match(/@(sha256:[a-f0-9]{64})$/)?.[1]
    if (!digest) throw new Error(`could not determine pushed manifest digest for ${target}`)
    return { digest, cleanup }
  } catch (error) {
    await cleanup().catch(() => undefined)
    throw error
  }
}
