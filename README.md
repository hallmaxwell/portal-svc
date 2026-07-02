# Portal-SVC

Welcome to **Portal-SVC**! This project provides a robust, auto-recovering background service daemon for network routing and proxying, built around `sing-box`.

To accommodate different environments, Portal-SVC is distributed in two distinct versions:
1. **Local Client (`portal-svc`)**: A system service manager designed to run directly on your host machine (Windows, Linux, macOS).
2. **Remote Node (`portal-svc` Docker Image)**: A lightweight, containerized version designed to run as a server-side node or inside isolated environments, deployed easily via `docker-compose`.

This guide will walk you through acquiring, initializing, and running both versions.

---

## 1. Acquiring the Software

### Local Client
The Local Client is provided as a pre-compiled standalone binary. You do not need any additional software to run it.
- **Download**: Head over to the [GitHub Releases](https://github.com/hallmaxwell/portal-svc/releases) page.
- Choose the archive that matches your operating system and architecture (e.g., `portal-svc-vX.Y.Z-windows-amd64.zip`, `portal-svc-vX.Y.Z-linux-amd64.tar.gz`).
- Extract the archive to a permanent location on your machine. You will find the executable named `portal-svc` (or `portal-svc.exe` on Windows).

### Remote Node (Docker Image)
The Remote Node is distributed as a Docker image hosted on the GitHub Container Registry (GHCR).
- **Pull the Image**:
  ```bash
  docker pull ghcr.io/hallmaxwell/portal-svc:latest
  ```

---

## 2. Installing the Local Client

For the Local Client, it is highly recommended to install the service first. Installing registers the binary with your operating system's service manager (e.g., systemd, Windows Services) and often places it in your system's PATH. This means you can call `portal-svc` globally from any terminal without needing to prepend `./`.

To install the service:
```bash
./portal-svc install
```
*(Note: On Windows, this will automatically request Administrator privileges if required; on Linux/macOS, you may need to prefix it with `sudo`)*.

---

## 3. Initialization: Generating Configuration

Both versions require an `.env` file containing secrets and a JSON template file for configuration. Fortunately, the binary includes a built-in command to generate these files for you.

Before starting the service, run the `generate` command in your working directory.

**For Local Client:**
*(If you installed the service in the previous step, you can just use `portal-svc`)*
```bash
portal-svc generate
```

**For Remote Node (via Docker):**
```bash
docker run --rm -v $(pwd):/app ghcr.io/hallmaxwell/portal-svc:latest generate
```

This command will:
1. Generate a secure `.env` file containing necessary cryptographic keys and identifiers.
2. Create a `templates/` directory containing the default configuration templates (e.g., `local_config.tmpl.json` or `remote_config.tmpl.json`).

*Note: You only need to do this once. Keep the generated `.env` file secure.*

---

## 4. Running the Local Client

With the service installed and configuration generated, you can now manage and start your local client.

### Interactive Tweaking
Before starting, you can interactively modify common configuration settings using a Terminal UI:
```bash
portal-svc tweak
```

### Service Management Commands
To manage the background service, use the following commands:

- **Start the service**:
  ```bash
  portal-svc start
  ```
- **Check the status**:
  ```bash
  portal-svc status
  ```
- **View logs** (useful for troubleshooting):
  ```bash
  portal-svc logs -f
  ```
- **Stop the service**:
  ```bash
  portal-svc stop
  ```
- **Uninstall the service** (if you wish to remove it from your system):
  ```bash
  portal-svc uninstall
  ```

---

## 5. Running the Remote Node (Docker)

The Remote Node is designed to run continuously in the background using Docker.

Because it handles low-level network routing, **it is highly recommended to run the container using Host Network Mode (`network_mode: host`)**. It also needs access to the `.env` file and `templates` directory you generated earlier.

The recommended and simplest way to run the Remote Node is via `docker-compose`.

### Using `docker-compose`

Create a `docker-compose.yml` file in your working directory with the following content:

```yaml
services:
  portal-node:
    image: ghcr.io/hallmaxwell/portal-svc:latest
    container_name: portal-node
    restart: always
    network_mode: host
    env_file: .env
    volumes:
      - ./templates/remote_config.tmpl.json:/app/templates/remote_config.tmpl.json
      - ./srs_cache:/app/srs
      - /etc/localtime:/etc/localtime:ro
```

**Explanation of the configuration:**
- `network_mode: host`: Shares the host's network stack with the container, essential for correct routing.
- `env_file: .env`: Loads the secrets you generated earlier.
- `volumes`:
  - Mounts your local template file into the container.
  - Caches downloaded routing rules (`srs_cache`) locally so they persist across container restarts.
  - Syncs the container's timezone with the host machine.

Then, simply start the service by running:
```bash
docker-compose up -d
```

To stop the service, run:
```bash
docker-compose down
```

---

## 6. Additional Utilities

Both versions support rendering templates for testing purposes. If you want to verify what the final JSON configuration will look like after environment variables are injected, use the `render` command:

```bash
# Local Client
portal-svc render --config templates/local_config.tmpl.json --out final_config.json

# Remote Node
docker run --rm \
  --env-file .env \
  -v $(pwd):/app \
  ghcr.io/hallmaxwell/portal-svc:latest render \
    --config /app/templates/remote_config.tmpl.json \
    --out /app/final_config.json
```
