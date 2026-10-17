/** Quote one host path as an argument for the actual local shell. */
export function quoteShellPath(path: string, shellPath = ''): string {
  if (/[\x00\r\n]/.test(path)) throw new Error('Shell paths cannot contain control characters')
  const shell = shellPath.toLowerCase()
  const windows = /^[a-z]:[\\/]/i.test(path) || path.startsWith('\\\\')
  if (/powershell|pwsh/.test(shell)) return "'" + path.replace(/'/g, "''") + "'"
  const unix = /wsl|bash|zsh|fish|msys|cygwin|mingw/.test(shell) || !windows
  if (unix) {
    if (/^[a-z]:[\\/]/i.test(path)) {
      path = (shell.includes('wsl') ? '/mnt/' : '/') + path[0].toLowerCase() + path.slice(2).replace(/\\/g, '/')
    }
    return shell.includes('fish')
      ? "'" + path.replace(/\\/g, '\\\\').replace(/'/g, "\\'") + "'"
      : "'" + path.replace(/'/g, "'\\''") + "'"
  }
  if (!/cmd|clink/.test(shell)) throw new Error('Choose a local shell before pasting a Windows host path.')
  // cmd expands %variables% and (with delayed expansion) !variables! even
  // inside quotes. Refuse those paths instead of silently changing the name.
  if (/[!%"]/.test(path)) throw new Error('This path cannot be pasted literally into cmd; use PowerShell or a file panel.')
  return '"' + path + '"'
}
