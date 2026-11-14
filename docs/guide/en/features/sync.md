# Cloud Sync

Supports **five cloud sync modes over Git / WebDAV / SFTP / S3 / SMB** with encrypted synchronization of uniTerm settings: the Git mode uses your own private repository (GitHub / GitLab / Gitee), the WebDAV mode works with any standard WebDAV service, the SFTP / SMB modes reuse servers you already have, and the S3 mode works with any S3-compatible object storage. Your data security stays fully under your control.

![Cloud Sync](/imgs/cloud_sync_light.webp)

## Quick Setup

### 1. Create a Private Repository + Access Token

#### GitHub

**Create a private repository:** Log in to GitHub → Click `+` in the top right → New repository → Enter a repository name → Choose **Private** → Create repository.

**Generate an access token:** Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token:
- Repository access: Choose **Only select repositories** and select the private repository you just created
- Permissions → Contents: Set to **Read and write**
- Copy the token after generation (it is shown only once)

#### GitLab

**Create a private repository:** Log in to GitLab → New project → Create blank project → Enter a project name → Set Visibility Level to **Private** → Create project.

**Generate an access token:** Settings → Access Tokens → Add new token:
- Token name: Enter a name
- Select scopes: Check **api**, or at least check **read_repository** + **write_repository**
- Copy the token after generation (it is shown only once)

#### Gitee

**Create a private repository:** Log in to Gitee → Click `+` in the top right → New Repository → Enter a repository name → Choose **Private** → Create.

**Generate an access token:** Click your avatar in the top right → Settings → Private Tokens → Generate New Token:
- Check **user_info** and **projects** permissions
- Copy the token after generation (it is shown only once)

#### WebDAV

The WebDAV mode needs no repository — just prepare a WebDAV service:

- **Server URL**: works with WebDAV-capable clouds (Nextcloud, Jianguoyun, InfiniCloud, ...) or a self-hosted service; the URL must include protocol and port
- **Account**: the username and password (app password) of that service

#### SFTP

The SFTP mode reuses any server you can log into over SSH:

- **Server**: `host[:port]` format, port defaults to 22
- **Account**: the username and password of that server

#### S3

The S3 mode works with any S3-compatible object storage (AWS S3, MinIO, Cloudflare R2, ...):

- **Endpoint**: e.g. `https://s3.amazonaws.com`
- **Access Key / Secret Key**: the access keys of that object storage
- **Bucket**: the bucket name (required)
- **Region**: e.g. `us-east-1`
- **URL Style**: Virtual-hosted or Path, depending on the provider

#### SMB

The SMB mode works with Windows shared folders or NAS devices:

- **Server**: `host[:port]` format, port defaults to 445
- **Share**: the SMB share name (required)
- **Domain**: optional, the Windows domain (e.g. `WORKGROUP`)
- **Account**: the username and password of that service

### 2. Configure uniTerm

1. Open Settings → Cloud Sync and click "Add Repository", choosing the sync mode **Git / WebDAV / SFTP / S3 / SMB**
2. Fill in the parameters for the chosen mode: Git — repository URL, username (optional), access token; WebDAV — server URL (with protocol and port), username, password; SFTP — server (`host[:port]`), username, password; S3 — endpoint, Access Key / Secret Key, bucket, region; SMB — server (`host[:port]`), share name, username, password
3. Set a **master password** (used to encrypt data; it is not uploaded to the repository or cloud drive)
4. Click OK, and uniTerm will automatically complete the initial synchronization
5. Tick the data to sync under "Sync Scope" as needed

::: warning Note
- Keep your token and master password safe. The token is shown only once
- The repository must be set to private, otherwise synchronized data will be publicly visible
- When syncing across multiple devices, enter the same repository URL, token, and master password on each device
:::

## Sync Operations

### Manual Sync

Click the "Sync Now" button, and uniTerm will automatically compare local and remote differences:
- **Local is newer → Push to remote** (upload)
- **Remote is newer → Pull to local** (download)
- **No differences → Already up to date**

### Auto Sync

When auto-sync is enabled:
- Sync automatically once at **startup**
- Upload automatically after **each modification** to connections, favorites, quick commands, tunnels, identities, or proxies

## Sync Contents

The following data is synchronized by default (stored encrypted):

- Connection list and groups
- Connection favorites
- AI configuration (model catalog and autonomous turn limit)
- Quick commands
- SSH tunnels
- Identities
- Proxies

Additional opt-in entries in the sync scope (off by default):

- **Skills** (skills.json): a complete bundle — skill directories (SKILL.md, references/, scripts/) plus preferences (enabled/locked/sort order) — so a new device gets the full set with one pull. Off by default because skills can carry multiple, larger files. Limits: 256KB per file, 4MB per bundle
- **Basic settings** (settings.json): theme, paths, shell, shortcuts, interface layout

Data that is NOT synchronized:

- Application personalization settings (when "Basic settings" is not ticked — local only)
- AI sessions (local only)
- Cloud sync configuration itself (configured independently per device)
- Terminal history

::: warning Skills sync notes
- Deleting a skill on another device never removes it from this machine — deletes do not propagate, protecting skills copied into the directory by hand
- System skills shipped with the app (`.system`) are not synced; they follow the app build on each device
:::

## Conflict Handling

Synchronization pulls from the remote before pushing, so conflicts generally do not occur when two machines are used alternately. When multiple devices modify the same data at the same time without pulling the latest version before pushing, uniTerm shows a conflict resolution dialog with two options:

- **Use Local Data** — Use the current device's data as authoritative, overwriting the remote repository
- **Use Remote Data** — Use the remote repository's data as authoritative, overwriting the current device

## Encryption & Security

- **Encryption Algorithm:** AES-256-GCM, using a random nonce for each encryption
- **Key Derivation:** The encryption key is derived from the master password using the PBKDF2-SHA256 algorithm
- **Key Storage:** The derived key is stored in the operating system keychain (Windows Credential Manager / macOS Keychain / Linux keyring)
- **Zero Trust:** All data is encrypted on the client side before uploading. The Git server only sees ciphertext

## Managing Repositories

### Modify Credentials

Click "Edit Repository" to modify the username and access token. The repository URL cannot be modified (you must unbind first and then reconfigure).

When modifying credentials, uniTerm automatically verifies that the new token can access the repository and that the master password can decrypt the remote data.

### Change Master Password

Click "Change Master Password" to change the encryption password. The key is re-derived using the same salt, and all data is re-encrypted and pushed.

### Unbind

Click "Unbind" to:
- Delete the local sync repository
- Clear the encryption key and token saved in the keychain
- Reset the sync configuration

> The remote repository will not be deleted. To delete it, go to the Git service platform and do so manually.

## Sync Status

The Settings page displays the time and status of the last synchronization:

- **Success** (green) — The most recent sync completed normally
- **Failed** (red) — Displays the specific error message (e.g. network timeout, invalid token, etc.)
