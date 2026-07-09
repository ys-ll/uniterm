# Server Monitor

The server monitoring feature connects to remote Linux servers via SSH to collect and display real-time system metrics, helping operators quickly assess server health.

> Note: Server monitoring only supports **Linux** remote servers, as it relies on the `/proc` filesystem and Linux-specific commands. SSH credentials (password or key) are required.

## Opening the Monitor

Once an SSH connection is established, the monitoring panel can be opened from several entry points:

| Entry Point | Action |
|------|------|
| Tab Context Menu | Right-click an SSH terminal tab → **Open Server Monitor** |
| Panel Menu | Click the `...` button on the panel header → **Open Server Monitor** |
| Sidebar Context Menu | Right-click an SSH connection → **Open Server Monitor** |

The server monitor opens as an independent tab alongside terminal, file transfer, and other tabs. It supports drag-to-reorder.

## Interface Overview

The monitoring panel is organized into six tabs, switchable via the top tab bar.

## Performance

Displays four core metrics — CPU, Memory, Disk, and Network — in real time, refreshing every second with 60-point line charts showing the last 1 minute of history.

### CPU

- Total usage percentage with line chart
- Core count
- Total process count and file handle count
- 1-minute, 5-minute, and 15-minute load averages

### Memory

- Total, used, available, and usage percentage
- Cached and buffers usage

### Disk

- Root filesystem (`/`) total, used, and usage percentage

### Network

- Receive and transmit rates in bytes per second, calculated from delta between consecutive samples

## Processes

Lists the top **30 processes** by CPU usage, refreshing every second.

| Feature | Description |
|------|------|
| Sorting | Default sorted by CPU descending |
| Search | Filter by process name, username, or PID |
| Pause | Click the pause button to stop refresh; click again to resume |
| Process Detail | Click any process row to open a detail drawer from the right |

### Process Detail

Clicking a process opens a slide-in drawer with the following information:

- **Basic Info** — PID, PPID, name, state, thread count, executable path, working directory, full command line, start time
- **File Descriptors** — Total count with breakdown (files, sockets, pipes, anonymous, devices)
- **Virtual Memory** — VmRSS, VmSize, VmPeak, VmData, VmStk, VmExe, VmLib
- **CPU & Context Switches** — CPU ticks, voluntary/involuntary context switches
- **I/O Statistics** — Read/write character and byte counts

### Sending Signals

At the bottom of the process detail drawer, you can send signals to the process:

| Signal | Number | Description |
|------|------|------|
| TERM | 15 | Graceful termination (default) |
| KILL | 9 | Force kill |
| HUP | 1 | Hangup, commonly used to reload configuration |
| INT | 2 | Interrupt, equivalent to `Ctrl+C` |

A confirmation dialog appears before sending. Force kill (KILL) includes an additional warning.

## Ports

Manual refresh. Lists all listening TCP/UDP ports.

| Column | Description |
|------|------|
| Protocol | TCP / UDP |
| Local Address | Listening address and port |
| Process | Process PID and name |

Data is collected via `ss -tulnp` (falls back to `netstat -tulnp` if unavailable).

## Disks

Manual refresh. Displays all block devices in a tree structure.

| Column | Description |
|------|------|
| Name | Device name; partitions are indented |
| Type | disk / part / rom |
| Size | Total device capacity |
| Mount Point | Mount path |
| Used / Usage | Used space and percentage |
| Media | HDD / SSD / ROM |
| Filesystem | ext4, xfs, etc. |
| UUID | Device UUID |
| Vendor / Model | Hardware information |

Data is collected via `lsblk -J` and `df -h`. Media type is determined by the `rota` flag (1 = HDD, 0 = SSD).

## Network Cards

Manual refresh. Lists all network interfaces.

| Column | Description |
|------|------|
| Name | Interface name (eth0, wlan0, etc.) |
| State | UP / DOWN |
| MAC | MAC address |
| Speed | Interface speed (Mbps) |
| Type | Physical / Bridge / Bond / Virtual / Loopback |
| Bond Master | Parent bond interface (if applicable) |
| IP Addresses | Bound IP addresses |

Data is collected via `ip -j link show` and `ip -j addr show`. Bond and bridge interfaces are automatically detected.

## System Info

Collected once upon connection. Displays static server configuration information.

| Category | Content |
|------|------|
| System | OS (from `/etc/os-release`), version, kernel (`uname -r`), hostname, timezone |
| Hardware | CPU model, core count, architecture (`uname -m`), CPU frequency, total memory, total disk |
| Network | Local IP |


::: tip Related
- [Remote Terminal](/en/connections/remote-terminal) — SSH connection configuration and usage
- [File Transfer](/en/connections/file-transfer) — SFTP file browser
:::
