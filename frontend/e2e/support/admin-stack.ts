import { createServer } from 'node:net'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import type { Runtime } from './runtime'

const execFileAsync = promisify(execFile)
const adminEmail = 'admin-e2e@grom.local'
const adminPassword = 'admin-e2e-password'
const maxOutput = 4 << 20

function repositoryRoot() {
  return resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
}

async function reservePort(): Promise<number> {
  return new Promise((resolvePort, reject) => {
    const server = createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        reject(new Error('could not reserve a loopback port'))
        return
      }
      server.close((error) => error ? reject(error) : resolvePort(address.port))
    })
  })
}

async function run(root: string, environment: NodeJS.ProcessEnv, ...args: string[]) {
  try {
    const { stdout, stderr } = await execFileAsync('docker', args, {
      cwd: root,
      env: environment,
      timeout: 6 * 60_000,
      maxBuffer: maxOutput,
    })
    return `${stdout}${stderr}`
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    throw new Error(`docker ${args.join(' ')} failed: ${detail}`)
  }
}

async function waitForReady(publicURL: string) {
  const deadline = Date.now() + 90_000
  let last = 'no response'
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${publicURL}/readyz`, { signal: AbortSignal.timeout(2_000) })
      last = String(response.status)
      if (response.ok) return
    } catch (error) {
      last = error instanceof Error ? error.message : String(error)
    }
    await new Promise((resolveDelay) => setTimeout(resolveDelay, 500))
  }
  throw new Error(`public Grom endpoint did not become ready: ${last}`)
}

export default async function globalSetup() {
  const root = repositoryRoot()
  const port = await reservePort()
  const runtimeDirectory = await mkdtemp(join(tmpdir(), 'grom-admin-e2e-'))
  const publicURL = `http://127.0.0.1:${port}`
  const project = `gromadmine2e${process.pid}${Date.now()}`
  const runtime: Runtime = { root, project, publicURL, runtimeDirectory }
  const runtimeFile = join(runtimeDirectory, 'runtime.json')
  process.env.GROM_ADMIN_E2E_RUNTIME_FILE = runtimeFile
  const environment = {
    ...process.env,
    GROM_BIND_ADDRESS: '127.0.0.1',
    GROM_HTTP_PORT: String(port),
    GROM_PUBLIC_URL: publicURL,
    GROM_DEPLOYMENT_PROFILE: 'development',
    GROM_SECURE_COOKIES: 'false',
    GROM_BOOTSTRAP_ADMIN_EMAIL: adminEmail,
    GROM_BOOTSTRAP_ADMIN_USERNAME: 'admin-e2e',
    GROM_BOOTSTRAP_ADMIN_PASSWORD: adminPassword,
    GROM_AUTH_FAILURE_LIMIT: '50',
    GROM_AUTH_FAILURE_WINDOW: '1m',
    GROM_AUTH_BLOCK_DURATION: '1m',
    GROM_REGISTRY_HTTP_SECRET: 'admin-e2e-http-secret',
  }
  const compose = ['compose', '--ansi', 'never', '--project-name', project, '--file', join(root, 'deploy/compose/docker-compose.yml')]
  await writeFile(runtimeFile, JSON.stringify(runtime), 'utf8')
  try {
    await run(root, environment, ...compose, 'up', '--build', '--detach')
    await waitForReady(publicURL)
  } catch (error) {
    await run(root, environment, ...compose, 'down', '--volumes', '--remove-orphans').catch(() => undefined)
    await rm(runtimeDirectory, { recursive: true, force: true })
    throw error
  }
  return async () => {
    try {
      await run(root, environment, ...compose, 'down', '--volumes', '--remove-orphans')
    } finally {
      await rm(runtimeDirectory, { recursive: true, force: true })
    }
  }
}

export const ADMIN_E2E_ADMIN = { email: adminEmail, password: adminPassword }
