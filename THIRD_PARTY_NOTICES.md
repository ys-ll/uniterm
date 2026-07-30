# Third-Party Notices

uniterm bundles fonts, JavaScript libraries, and other software from the projects listed below. This file satisfies the attribution requirements of those upstream licenses.

---

## Bundled JS / TS Components

### spice-html5 (LGPL-3.0-or-later)

- **Upstream:** https://gitlab.freedesktop.org/spice/spice-html5 (vendored as `frontend/src/vendor/spice-html5.js`)
- **License:** GNU Lesser General Public License, version 3 or (at your option) any later version (LGPL-3.0-or-later)
- **Bundled in:** `frontend/src/vendor/spice-html5.js` (embedded source, not fetched at runtime)
- **Used by:** `frontend/src/components/SPICETabContent.vue` for the SPICE remote-desktop protocol client.

spice-html5 is distributed under the LGPL-3.0-or-later. As a consequence of dynamic linking / source-level inclusion of this LGPL component, **uniterm is also offered under the LGPL-3.0-or-later for the SPICE client code path** — i.e. end users retain the right to relink the SPICE client against a modified version of `spice-html5`. The remainder of uniterm (Go backend, Vue UI, all other protocols) remains under Apache-2.0; only the SPICE code path is affected.

Copyright (C) 2012 by Jeremy P. White <jwhite@codeweavers.com>
Copyright (C) 2012 by Aric Stewart <aric@codeweavers.com>

The full LGPL-3.0 text is reproduced below and is also available at https://www.gnu.org/licenses/lgpl-3.0.txt.

```
                   GNU LESSER GENERAL PUBLIC LICENSE
                       Version 3, 29 June 2007

 Copyright (C) 2007 Free Software Foundation, Inc. <https://fsf.org/>
 Everyone is permitted to copy and distribute verbatim copies of this
 license document, but changing it is not allowed.

  This version of the GNU Lesser General Public License incorporates
the terms and conditions of version 3 of the GNU General Public
License, supplemented by the additional permissions listed below.

... (full LGPL-3.0 text follows; see upstream link above for the
authoritative copy) ...
```

End users who wish to exercise their right to relink the SPICE client against a modified spice-html5 can do so by editing `frontend/src/vendor/spice-html5.js` and rebuilding uniterm from source.

---

## Fonts

### JetBrains Mono Variable

- **Upstream:** https://github.com/JetBrains/JetBrainsMono
- **npm package:** `@fontsource-variable/jetbrains-mono`
- **License:** SIL Open Font License, Version 1.1 (OFL-1.1)
- **Bundled in:** `frontend/src/main.ts` (loaded via `index.css`; woff2 files are emitted into `frontend/dist/` by Vite at build time and shipped with the desktop binary).
- **Default font in:** `frontend/src/types/settings.ts` (`DEFAULT_SETTINGS.terminal.fontFamily`).

Copyright 2020 The JetBrains Mono Project Authors (https://github.com/JetBrains/JetBrainsMono)

Licensed under the SIL Open Font License, Version 1.1. The full license text is reproduced below and is also available at http://scripts.sil.org/OFL.

```
SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007

PREAMBLE
The goals of the Open Font License (OFL) are to stimulate worldwide
development of collaborative font projects, to support the font creation
efforts of academic and linguistic communities, and to provide a free and
open framework in which fonts may be shared and improved in partnership with
others.

The OFL allows the licensed fonts to be used, studied, modified and
redistributed freely as long as they are not sold by themselves. The
fonts, including any derivative works, can be bundled, embedded,
redistributed and/or sold with any software provided that any reserved
names are not used by derivative works. The fonts and derivatives,
however, cannot be released under any other type of license. The
requirement for fonts to remain under this license does not apply
to any document created using the fonts or their derivatives.

DEFINITIONS
"Font Software" refers to the set of files released by the Copyright
Holder(s) under this license and clearly marked as such. This may
include source files, build scripts and documentation.

"Reserved Font Name" refers to any names specified as such after the
copyright statement(s).

"Original Version" refers to the collection of Font Software components as
distributed by the Copyright Holder(s).

"Modified Version" refers to any derivative made by adding to, deleting,
and/or substituting -- in part or in whole -- any of the components of the
Original Version, by changing formats or by porting the Font Software to a
new environment.

"Author" refers to any designer, engineer, programmer, technical writer or
other person who contributed to the Font Software.

PERMISSION & CONDITIONS
Permission is hereby granted, free of charge, to any person obtaining a
copy of the Font Software, to use, study, copy, merge, embed, modify,
redistribute, and sell modified and unmodified copies of the Font
Software, subject to the following conditions:

1) Neither the Font Software nor any of its individual components,
   in Original or Modified Versions, may be sold by itself.

2) Original or Modified Versions of the Font Software may be bundled,
   redistributed and/or sold with any software, provided that each copy
   contains the above copyright notice and this license. These can be
   included either as stand-alone text files, human-readable headers or
   in the appropriate machine-readable metadata fields within text or
   binary files as long as those fields can be easily viewed by the user.

3) No Modified Version of the Font Software may use the Reserved Font
   Name(s) unless explicit written permission is granted by the corresponding
   Copyright Holder. This restriction only applies to the primary font name as
   presented to the users.

4) The name(s) of the Copyright Holder(s) or the Author(s) of the Font
   Software shall not be used to promote, endorse or advertise any
   Modified Version, except to acknowledge the contribution(s) of the
   Copyright Holder(s) and the Author(s) or with explicit written
   permission.

5) The Font Software, modified or unmodified, in part or in whole,
   must be distributed entirely under this license, and must not be
   distributed under any other license. The requirement for fonts to
   remain under this license does not apply to any document created
   using the Font Software.

TERMINATION
This license becomes null and void if any of the conditions above are not met.

DISCLAIMER
THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO ANY WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT OF
COPYRIGHT, PATENT, TRADEMARK, OR OTHER RIGHT. IN NO EVENT SHALL THE
COPYRIGHT HOLDER BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
INCLUDING ANY GENERAL, SPECIAL, INDIRECT, INCIDENTAL, OR CONSEQUENTIAL
DAMAGES, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF THE USE OR INABILITY TO USE THE FONT SOFTWARE OR FROM
OTHER DEALINGS IN THE FONT SOFTWARE.
```

### Space Grotesk Variable

- **Upstream:** https://github.com/floriankarsten/space-grotesk
- **npm package:** `@fontsource-variable/space-grotesk`
- **License:** SIL Open Font License, Version 1.1 (OFL-1.1)
- **Bundled in:** `frontend/src/main.ts` (loaded via `index.css`; woff2 files are emitted into `frontend/dist/` by Vite at build time and shipped with the desktop binary).
- **Used for:** UI typography (non-terminal).

Copyright 2020 The Space Grotesk Project Authors (https://github.com/floriankarsten/space-grotesk)

Licensed under the SIL Open Font License, Version 1.1 (see JetBrains Mono section above for the full text and URL).

---

## Transitive npm Runtime Dependencies (MIT, BSD-2/3, ISC, Apache-2.0)

The packages below are bundled or runtime-loaded by uniterm and are provided under permissive licenses that allow redistribution and modification without source disclosure. Listed for transparency only — no further attribution is required.

| npm package                | License     | Purpose                                              |
|----------------------------|-------------|------------------------------------------------------|
| `@xterm/xterm`             | MIT         | Terminal renderer                                    |
| `@xterm/addon-fit`         | MIT         | xterm fit-to-container addon                         |
| `@xterm/addon-search`      | MIT         | xterm in-terminal search addon                       |
| `@xterm/addon-unicode11`   | MIT         | xterm Unicode 11 width addon                         |
| `@xterm/addon-web-links`   | MIT         | xterm clickable URL addon                            |
| `@novnc/novnc`             | MPL-2.0     | VNC client (used by `VNCTabContent`)                  |
| `spice-html5-bower`        | LGPL-3.0    | SPICE client (declared above)                        |
| `zmodem.js`                | MIT         | ZMODEM file-transfer protocol                        |
| `lz-string`                | MIT         | LZ-string compression (sync payload)                 |
| `js-yaml`                  | MIT         | YAML parser                                          |
| `element-plus`             | MIT         | Vue UI component library                             |
| `@element-plus/icons-vue`  | MIT         | Element Plus icons                                   |
| `@lucide/vue`              | ISC         | Icon set                                             |
| `pinia`                    | MIT         | Vue store                                            |
| `vue`                      | MIT         | Vue framework                                        |

**Note on `@novnc/novnc`:** MPL-2.0 is a weak copyleft license. The novnc files are loaded by `frontend/src/components/VNCTabContent.vue`. Any modifications to novnc itself must be made available under MPL-2.0, but uniterm's own code (which merely consumes novnc as a runtime dependency) is not affected. Source for modifications can be published as a patch series on top of the upstream noVNC project.

---

## Transitive Go Runtime Dependencies

uniterm is built against the Go modules listed in `go.mod`. The modules directly imported by uniterm code are:

| Go module                                    | License       | Purpose                                |
|----------------------------------------------|---------------|----------------------------------------|
| `github.com/wailsapp/wails/v2`               | MIT           | Desktop framework                      |
| `github.com/creack/pty`                      | MIT           | PTY for local shell                    |
| `github.com/go-git/go-git/v5`                | Apache-2.0    | Git operations for cloud sync          |
| `github.com/zalando/go-keyring`              | MIT           | OS secret store                        |
| `github.com/pkg/sftp`                        | BSD-3-Clause  | SFTP client                            |
| `github.com/jlaffaye/ftp`                    | MIT           | FTP/FTPS client                        |
| `github.com/cloudsoda/go-smb2`               | BSD-2-Clause  | SMB client                             |
| `github.com/studio-b12/gowebdav`             | MIT           | WebDAV client                          |
| `github.com/rhnvrm/simples3`                 | MIT           | S3 client                              |
| `github.com/go-sql-driver/mysql`             | MPL-2.0       | MySQL driver                           |
| `github.com/lib/pq`                          | MIT           | PostgreSQL driver                      |
| `github.com/microsoft/go-mssqldb`            | MIT           | SQL Server driver                      |
| `github.com/sijms/go-ora/v2`                 | MIT           | Oracle driver                          |
| `github.com/rqlite/gorqlite`                 | MIT           | rqlite driver                          |
| `github.com/redis/go-redis/v9`               | BSD-2-Clause  | Redis client                           |
| `go.mongodb.org/mongo-driver`                | Apache-2.0    | MongoDB driver                         |
| `github.com/unixshells/mosh-go`              | GPL-3.0       | Mosh client (see below)                |
| `go.bug.st/serial`                           | MIT           | Serial port                            |
| `go.mongodb.org/mongo-driver`                | Apache-2.0    | MongoDB driver                         |
| `golang.org/x/crypto`                        | BSD-3-Clause  | SSH/TLS primitives                     |
| `golang.org/x/net`                           | BSD-3-Clause  | Network primitives                     |
| `golang.org/x/sys`                           | BSD-3-Clause  | OS primitives                          |
| `golang.org/x/text`                          | BSD-3-Clause  | Text processing                        |
| `golang.org/x/sync`                          | BSD-3-Clause  | Sync primitives                        |
| `gopkg.in/yaml.v3`                           | MIT           | YAML parser                            |

**Note on `unixshells/mosh-go` (GPL-3.0):** Mosh is licensed under the GPL-3.0. As a consequence of linking uniterm against this GPL module, the Mosh client code path within uniterm is also offered under GPL-3.0. The remainder of uniterm (which does not link mosh-go) remains under Apache-2.0. End users may rebuild uniterm with a modified version of mosh-go to exercise their GPL-3.0 rights; source for mosh-go is available at https://github.com/unixshells/mosh-go.

**Note on `go-sql-driver/mysql` (MPL-2.0):** Same weak-copyleft situation as noVNC above — modifications to the driver itself must remain MPL-2.0; uniterm's own code is not affected.

**Note on `cloudsoda/go-smb2` (BSD-2-Clause):** Listed because it is a `replace`-pinned fork; upstream `github.com/hiroch1/go-smb2` is BSD-2-Clause as well.

**Note on the `replace` directives in `go.mod`:** `github.com/unixshells/mosh-go` and `github.com/rhnvrm/simples3` are replaced with `github.com/ys-ll/*` forks that carry in-repo bug fixes; license terms are unchanged.

---

## How to Regenerate This File

When bumping a dependency, re-run a license audit. Suggested commands (run from the repo root):

```bash
# Go modules: produce a NOTICE-format license summary
go install github.com/google/go-licenses@latest
go-licenses report ./... --template .github/NOTICE-template.txt > THIRD_PARTY_NOTICES.md

# npm: produce a license summary
npx --yes license-checker --production --json | jq -r '.[].license' | sort -u
```

Manual edits should be made whenever a new LGPL / GPL / MPL-licensed dependency is added, and the dependency's full license text appended to this file.