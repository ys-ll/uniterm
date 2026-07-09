# File Transfer

uniTerm has a built-in dual-pane file browser that supports multiple file transfer protocols, providing a unified file management interface.

## Supported Protocols

### SFTP

A secure file transfer protocol based on SSH. Connection parameters are shared with SSH -- host, port (default 22), username, password or key authentication.

### FTP / FTPS

Traditional FTP protocol and encrypted FTPS.

| Parameter | Description |
|------|------|
| Host | FTP server address |
| Port | Default 21 |
| Username / Password | Login credentials |
| Encryption | Optional FTPS (explicit/implicit TLS) |

### SMB

Windows file sharing protocol (Samba), used for connecting to LAN shared folders or NAS.

| Parameter | Description |
|------|------|
| Host | SMB server address |
| Port | Default 445 |
| Share Path | Shared directory path |
| Username / Password | Login credentials |
| Domain | Optional, for Windows domain environments |

### WebDAV

An HTTP-based file management protocol, commonly used for NextCloud, ownCloud, and other cloud storage services.

| Parameter | Description |
|------|------|
| URL | WebDAV service address |
| Username / Password | Login credentials |

### S3

Amazon S3 API-compatible object storage. Also supports MinIO, Alibaba Cloud OSS, and other compatible services.

| Parameter | Description |
|------|------|
| Endpoint | Service endpoint address |
| Access Key | Access key ID |
| Secret Key | Secret access key |
| Bucket | Bucket name |
| Region | Region (optional) |


## File Browser

Once connected, all protocols share the same file browser interface -- a dual-pane layout with the local filesystem on the left and the remote target on the right.

![File Transfer](/imgs/sftp_light.png)

### Dual-Pane Layout

The left pane shows the local filesystem and the right pane shows the remote target. The divider between them can be dragged to adjust the ratio. Each side has its own path bar and toolbar at the top.

### File Operations

- **Upload** -- Drag selected local files to the remote side, drag files from the desktop/file explorer into the window, or click the upload button in the toolbar
- **Download** -- Drag selected remote files to the local side, drag to the desktop/file explorer, or click the download button in the toolbar
- **Multi-select** -- Hold `Ctrl` to select multiple files individually, `Shift` for range selection
- **New Folder** -- Create a new directory at the current location
- **Delete** -- Delete selected files/folders (supports batch operations)
- **Rename** -- Right-click or click an already-selected filename to rename
- **Copy / Move** -- Copy or move files to another directory within the same side

### Context Menu

Right-click in the file list to open a context menu with common operations such as new folder, delete, rename, refresh, and copy path.

### Path Navigation

- **Path Bar** -- Displays the full path of the current directory. Click any directory segment to quickly jump to it
- **Manual Input** -- Type a path directly in the path bar and press Enter to navigate
- **Path Bookmarks** -- Bookmark frequently used paths. Click the star icon to add, and jump with one click from the bookmark list
- **Go Up** -- Click `..` or the toolbar button to go to the parent directory

### File List

- **List View** -- Displays file name, size, modification time, permissions, and other information in table format
- **Sorting** -- Click column headers to sort by name, size, modification time, etc. in ascending or descending order
- **Hidden Files** -- Toggle whether to show hidden files starting with `.`
- **Refresh** -- Manually refresh the current directory contents

### Text Editor

Double-click a text file to open it in the built-in editor, which supports character encoding switching, line ending type switching (LF/CRLF), and saving to the remote target.

### Transfer Management

- **Transfer Progress** -- The bottom status bar shows the current transfer progress
- **Transfer Queue** -- Multiple transfer tasks are queued for execution. Individual tasks can be paused or cancelled.


::: tip Related
- [Remote Terminal](/en/connections/remote-terminal) -- SSH connections (SFTP is based on SSH)
:::
