import { readFile } from 'node:fs/promises'

export type Runtime = { root: string; project: string; publicURL: string; runtimeDirectory: string }

export async function readRuntime(): Promise<Runtime> {
  const path = process.env.GROM_ADMIN_E2E_RUNTIME_FILE
  if (!path) throw new Error('admin E2E runtime file was not configured')
  return JSON.parse(await readFile(path, 'utf8')) as Runtime
}
