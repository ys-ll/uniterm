# SSH Tunnel

uniTerm has a built-in SSH Tunnel Manager that supports three forwarding modes. You can create, manage, and monitor multiple tunnel connections.

![SSH Tunnel](/imgs/ssh_tunnel_light.webp)

## Tunnel Manager

SSH tunnels are managed in a dedicated sidebar tab, as well as in Settings → Tunnels. Click the tunnel icon in the left sidebar to open the tunnel panel and view all created tunnels along with their running status. A search box is provided at the top of the panel.

- **Status Indicator** — Green means running; yellow means connecting or waiting to reconnect; gray means stopped; red means an error (hover for error details)
- **Right-Click Menu** — Right-click a tunnel item to start, stop, edit, or delete the tunnel (stop running, connecting, or reconnecting tunnels before editing)
- **Drag-and-Drop Groups** — Drag tunnel items into groups for organized management
- **Credential Prompt** — When starting an exit SSH connection for a tunnel without saved credentials, a dialog prompts for them instead of failing outright
- **Start Failure Hints** — When a tunnel fails to start, the actual reason is reported, along with a configuration test and a sample SSH command

## Three Forwarding Modes

### Local Forwarding (-L)

Map a remote server's port to your local machine, accessing the remote service through a local port.

**Typical Scenario**: A remote database (e.g., MySQL :3306) is not publicly accessible. With local forwarding mapping it to `localhost:3307`, local tools can connect directly.

```
Local Port :3307  →  SSH Tunnel  →  Remote Database :3306
```

### Remote Forwarding (-R)

Expose a local port to a remote server, allowing remote hosts to access your local service through their local port.

**Typical Scenario**: A locally developed service needs to be accessed by a remote server or external users, such as exposing a local web app to a public server through remote forwarding.

```
Remote Port :8080  →  SSH Tunnel  →  Local Service :3000
```

### Dynamic Forwarding (-D, SOCKS5)

Set up a local SOCKS5 proxy, allowing browsers or other applications to access all services within the remote network through the proxy.

**Typical Scenario**: Access all internal web systems, APIs, etc. within a corporate network through a jump host, without configuring individual port forwards.

```
Browser  →  SOCKS5 :1080  →  SSH Tunnel  →  All Services in Remote Network
```

## Creating a Tunnel

1. Click **New Tunnel** in the tunnel panel
2. Select an **SSH Connection** as the jump host (requires an SSH connection to be created in the connection list first)
3. Choose a **Forwarding Mode**: Local, Remote, or Dynamic
4. Fill in the local address/port and target address/port based on the mode (Dynamic mode only requires a local port)
5. Optional: Enable **Auto-Start** to automatically connect the tunnel on application launch
6. Optional: Enable **Auto-Reconnect**, set the base retry interval (120 seconds by default, configurable from 1 to 86400 seconds), and set random jitter (0–100%, default 0%)

## Auto-Start

When auto-start is enabled, the tunnel will automatically connect when uniTerm starts. This is useful for tunnels that need to remain connected at all times, such as a SOCKS5 proxy for a development environment.

## Auto-Reconnect

Auto-reconnect is configured per tunnel and works with local, remote, and dynamic forwarding. When enabled, an unexpected SSH disconnect or a failed connection attempt puts the tunnel into **Waiting to reconnect**. It retries after the configured interval and keeps retrying if an attempt fails. The delay starts when a disconnect is detected or a connection attempt fails.

With jitter set to 0%, the interval is fixed. For example, a base interval of 120 seconds with ±20% jitter gives each retry a fresh random delay between 96 and 144 seconds. Random delays are always at least 1 second.

Clicking **Stop**, deleting the tunnel, or quitting uniTerm cancels automatic retries. Click **Start** to resume. Auto-reconnect and auto-start can be enabled independently; testing a configuration never starts background retries.

Automatic connections require available SSH credentials. Temporary credentials entered for a manual start are reused only during that tunnel's current lifecycle and are not saved to disk by auto-reconnect. Connecting after an application restart still requires saved credentials.

## Multi-hop connections on macOS and Windows

Both desktop platforms use the same SSH jump-host configuration. The **SSH Tunnel** field in an ordinary connection follows the selected SSH connection's own jump host recursively. Each hop uses its own username, password, private key, or keystore identity.

### Example: client → A → B → C

| Saved connection | Example host and port | Advanced settings → Network → SSH Tunnel |
| --- | --- | --- |
| A (SSH entry gateway) | `gateway.example.com:22` | None; reachable directly from the client |
| B (SSH internal gateway) | `10.0.0.10:22` | A |
| C (target SSH server) | `10.0.1.20:22` | B |

1. Create and save SSH connection A with A's credentials.
2. Create SSH connection B, select A in **Advanced settings → Network → SSH Tunnel**, enter B's credentials, and save it.
3. Create target connection C with C's actual address and credentials. Select B as its SSH tunnel.
4. Click **Test Connection**, then connect to C. uniTerm opens A, then B, then the target connection. There is no need to open gateway terminal tabs first.

B's hostname is resolved from A; C's hostname is resolved from B. Keep C's actual address in the target connection. For a single jump host, omit B and select A on C. The same configuration applies to TCP connections that expose the SSH Tunnel option, such as SFTP and RDP; use each target protocol's own port and credentials.

### macOS

Configure A, B, and C in the macOS app and connect to C. The client uses built-in SSH support; enabling macOS Remote Login is not required to use external gateways.

Private-key paths must point to files on the Mac. SSH Agent authentication uses `SSH_AUTH_SOCK`, which must be available to the app. After syncing a connection from Windows, update local key paths or use inline key text or a keystore identity.

### Windows

Use the same A, B, and C configuration in the Windows app. Client-side chaining does not require Windows OpenSSH Server or WSL.

Private-key paths must point to files on the Windows computer. SSH Agent authentication tries Pageant first, then Windows OpenSSH Agent. After syncing a connection from macOS, update local key paths or use inline key text or a keystore identity.

### Lifecycle and troubleshooting

- An ordinary connection opens its jump-host chain when connecting to the target. Closing the target session or quitting the app closes all hops. The chain does not start independently on application launch.
- Provide credentials for every hop. Save upstream gateway credentials, reference keystore identities, or configure an SSH agent in advance; the current credential dialog does not prompt separately for every upstream gateway.
- A failed hop closes all earlier connections. Check the host identified by the error and that hop's credentials.
- A missing-hop error means a referenced connection cannot be found. A `loop` error means the saved references contain a cycle, such as A → B → A.
- Gateway SSH servers must allow TCP forwarding. Using uniTerm's embedded SSH server as a gateway requires a version containing [PR #1406](https://github.com/ys-ll/uniterm/pull/1406) and manually starting **Local Services → SSH/SFTP** on that gateway. Embedded-server forwarding and client-side multi-hop connections are separate capabilities.

### Standalone tunnels

The standalone Tunnel Manager also follows saved jump-host references. Selecting B as the SSH connection in this example opens `client → A → B`; B then forwards to the target service. Local, remote, and dynamic forwarding retain their respective semantics.

Use an ordinary connection to open a target session inside uniTerm. Use a standalone tunnel to expose a local port or SOCKS5 proxy to other applications. Standalone tunnels can separately enable **Auto-Start**.

## Reference products

This comparison concerns client-side jump hosts, not whether each product provides an SSH server.

| Product | Official configuration | uniTerm equivalent |
| --- | --- | --- |
| Xshell | Select `JUMPHOST` in proxy settings and chain gateways with `Next proxy` | Set the preceding SSH tunnel on each saved SSH connection; select the hop nearest the target on the target connection |
| MobaXterm | Configure `SSH gateway` in session `Network settings`; multiple SSH jump hosts are supported | Reference a saved jump-host chain from an ordinary connection or standalone tunnel |

Official references: [Xshell Proxy Setting](https://netsarang.atlassian.net/wiki/spaces/ENSUP/pages/2237304738/Proxy+Setting), [MobaXterm Network settings](https://mobaxterm.mobatek.net/documentation.html#4_10), and [MobaXterm multi-hop changelog](https://mobaxterm.mobatek.net/download-home-edition.html).

---

::: tip Related
- [Remote Terminal](/en/connections/remote-terminal) — SSH connection configuration
- [Workspace](/en/connections/workspace) — Sidebar tab management
- [Server Monitor](/en/connections/server-monitor) — Real-time server monitoring via SSH
:::
