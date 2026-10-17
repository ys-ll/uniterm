import { execFileSync } from 'node:child_process'
import { expect, it } from 'vitest'
import { quoteShellPath } from './shellPath'

it('pastes Unicode, spaces, quotes and shell metacharacters as one literal path', () => {
  const path = "/saved/访客's $(touch nope); [data].bin"
  for (const shell of ['/bin/sh', '/bin/bash', '/bin/zsh']) {
    expect(execFileSync(shell, ['-c', `printf '%s' ${quoteShellPath(path, shell)}`], { encoding: 'utf8' })).toBe(path)
  }
  expect(quoteShellPath("C:\\saved\\a's b.bin", 'pwsh.exe')).toBe("'C:\\saved\\a''s b.bin'")
  expect(quoteShellPath("/saved/a's b.bin", 'pwsh')).toBe("'/saved/a''s b.bin'")
  expect(quoteShellPath('C:\\saved\\a b.bin', 'cmd.exe')).toBe('"C:\\saved\\a b.bin"')
  expect(quoteShellPath("C:\\saved\\a's b.bin", 'wsl://Ubuntu')).toBe("'/mnt/c/saved/a'\\''s b.bin'")
  expect(quoteShellPath('C:\\saved\\a b.bin', 'git-bash.exe')).toBe("'/c/saved/a b.bin'")
  expect(() => quoteShellPath('C:\\saved\\%PATH%.bin', 'cmd.exe')).toThrow('literally')
})
