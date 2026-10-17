import { Call } from '@wailsio/runtime'

export interface WebAccessConfig {
  enabled: boolean
  host: '127.0.0.1' | '0.0.0.0'
  startPort: number
}

export interface WebAccessStatus {
  config: WebAccessConfig
  running: boolean
  actualPort: number
  urls: string[]
  tokenSet: boolean
  instanceId: string
  error: string
}

// Keep independent builds working until bindings are regenerated centrally.
// main.App is the verified Wails service FQN (not the module import path).
export const getWebAccessStatus = (): Promise<WebAccessStatus> => Call.ByName('main.App.GetWebAccessStatus')
export const configureWebAccess = (config: WebAccessConfig, token: string): Promise<WebAccessStatus> =>
  Call.ByName('main.App.ConfigureWebAccess', config, token)
export const generateWebAccessToken = (): Promise<string> => Call.ByName('main.App.GenerateWebAccessToken')
