# uniTerm Web (Docker, multi-user deployment)

Deploy uniTerm as a browser-accessible, multi-user service: every user gets an
isolated workspace (their own connections, SSH sessions, keys and data), hidden
from other users.

## Quick start

```bash
# Set the initial admin password (first startup only; leave empty to
# auto-generate a random one, printed in the container logs)
export UNITERM_ADMIN_PASSWORD='YourStrongPassword_2026!'

cd deploy/docker && docker compose up -d
```

Open `http://localhost:8080` in your browser and sign in with the admin
account.

### Using the Docker Hub image

Skip the build and pull the published image:

```bash
docker run -d --name uniterm-web \
  -p 8080:8080 \
  -v uniterm-data:/data \
  -e UNITERM_ADMIN_PASSWORD='YourStrongPassword_2026!' \
  --restart unless-stopped \
  uniterm/uniterm:latest
```

### Building the image yourself

Run from the repository root:

```bash
docker build -t uniterm/uniterm:latest .
```

## Data persistence

All data (accounts, per-user configuration, keys and personal data) lives in
the data directory — upgrading or recreating the container keeps everything as
long as the same directory is mounted:

- compose deployment: defaults to `deploy/docker/uniterm-data/`; point
  `UNITERM_DATA_HOST` elsewhere if needed;
- `docker run` deployment: pass `-v <host-directory>:/data`.

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `UNITERM_ADMIN_PASSWORD` | random | Initial admin password (first startup only; change it later in the UI) |
| `UNITERM_ADMIN_USER` | `admin` | Initial admin username |
| `UNITERM_DATA_HOST` | `./uniterm-data` | Host-side data directory |
| `UNITERM_WORKER_IDLE_MINUTES` | `60` | Minutes of inactivity before a user workspace is recycled; it restarts automatically on the next visit |
| `UNITERM_COOKIE_SECURE` | `false` | Set to `true` when deployed behind an HTTPS reverse proxy |
| `UNITERM_ALLOWED_ORIGINS` | empty | Public origins when deployed behind a reverse proxy (comma-separated) |

## User management (admin)

Sign in and open "User management" from the avatar menu in the top-right
corner:

- **Add user**: pick a username and an initial password (at least 6
  characters).
- **Reset password**: set a new password for a user. Note: the login password
  doubles as the key that encrypts that user's data, so resetting it **wipes
  all of that user's secret data** and their workspace starts from scratch.
  When users change their own password from the avatar menu, their data is
  migrated seamlessly instead.
- **Delete user**: removes the account together with all of its data.

## Upgrading

1. Back up the data directory (`deploy/docker/uniterm-data/` for compose
   deployments).
2. Pull (or rebuild) the new image and recreate the container with the same
   data-directory mount.

## Offline image import

Air-gapped environments can download
`uniterm-docker-<arch>-<version>.tar.gz` from the release page (pick amd64 or
arm64), import it, and start as usual:

```bash
docker load -i uniterm-docker-amd64-<version>.tar.gz
```

## Security notes

- **Always deploy behind an HTTPS reverse proxy** on public networks, and set
  `UNITERM_COOKIE_SECURE=true`.
- The login endpoint is protected against brute force: 5 consecutive failures
  lock the account for 10 minutes.
- Container-local terminals are unavailable: a local shell equals root in
  the container, which is a deliberate safety design for multi-user
  deployments.
