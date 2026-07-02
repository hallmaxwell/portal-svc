# Portal-SVC

Welcome to **Portal-SVC**! This project provides a robust, auto-recovering background service daemon for network routing and proxying, built around `sing-box`.

To accommodate different environments, Portal-SVC is distributed in two distinct versions:
1. **Local Client (`portal-local`)**: A system service manager designed to run directly on your host machine (Windows, Linux, macOS).
2. **Remote Node (`portal-svc` Docker Image)**: A lightweight, containerized version designed to run as a server-side node or inside isolated environments.

This guide will walk you through acquiring, initializing, and running both versions.

---

## 1. Acquiring the Software

### Local Client
The Local Client is provided as a pre-compiled standalone binary. You do not need any additional software to run it.
- **Download**: Head over to the [GitHub Releases](https://github.com/hallmaxwell/portal-svc/releases) page.
- Choose the archive that matches your operating system and architecture (e.g., `portal-svc-vX.Y.Z-windows-amd64.zip`, `portal-svc-vX.Y.Z-linux-amd64.tar.gz`).
- Extract the archive to a permanent location on your machine. You will find the executable named `portal-local` (or `portal-local.exe` on Windows).

### Remote Node (Docker Image)
The Remote Node is distributed as a Docker image hosted on the GitHub Container Registry (GHCR).
- **Pull the Image**:
  ```bash
  docker pull ghcr.io/hallmaxwell/portal-svc:latest
  ```

---

## 2. Initialization: Generating Configuration

Both versions require an `.env` file containing secrets and a JSON template file for configuration. Fortunately, the binary includes a built-in command to generate these files for you.

Before starting the service for the first time, run the `generate` command in your working directory.

**For Local Client:**
```bash
./portal-local generate
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

## 3. Running the Local Client

The Local Client acts as a system service. It can install itself so that it starts automatically when your machine boots.

Here are the primary commands you will use:

### Interactive Tweaking
Before starting, you can interactively modify common configuration settings using a Terminal UI:
```bash
./portal-local tweak
```

### Service Management Commands
To manage the background service, use the following commands. *(Note: On Windows, these commands will automatically request Administrator privileges if required; on Linux/macOS, you may need to prefix them with `sudo`)*.

- **Install the service** (registers it with your OS, e.g., systemd or Windows Services):
  ```bash
  ./portal-local install
  ```
- **Start the service**:
  ```bash
  ./portal-local start
  ```
- **Check the status**:
  ```bash
  ./portal-local status
  ```
- **View logs** (useful for troubleshooting):
  ```bash
  ./portal-local logs -f
  ```
- **Stop the service**:
  ```bash
  ./portal-local stop
  ```
- **Uninstall the service**:
  ```bash
  ./portal-local uninstall
  ```

---

## 4. Running the Remote Node (Docker)

The Remote Node is designed to run continuously in the background using Docker.

Because it handles low-level network routing, **it is highly recommended to run the container using Host Network Mode (`--network host`)**. It also needs access to the `.env` file and `templates` directory you generated earlier.

### Using `docker run` (Detailed)

To run the container directly via the CLI, use the following command. Note the specific volume mounts required:

```bash
docker run -d \
  --name portal-node \
  --restart always \
  --network host \
  --env-file .env \
  -v $(pwd)/templates/remote_config.tmpl.json:/app/templates/remote_config.tmpl.json \
  -v $(pwd)/srs_cache:/app/srs \
  -v /etc/localtime:/etc/localtime:ro \
  ghcr.io/hallmaxwell/portal-svc:latest
```

**Explanation of the arguments:**
- `-d`: Runs the container in the background (detached mode).
- `--network host`: Shares the host's network stack with the container, essential for correct routing.
- `--env-file .env`: Loads the secrets you generated earlier.
- `-v $(pwd)/templates/...`: Mounts your local template file into the container.
- `-v $(pwd)/srs_cache:/app/srs`: Caches downloaded routing rules locally so they persist across container restarts.
- `-v /etc/localtime...`: Syncs the container's timezone with the host machine.

### Using `docker-compose`

For a more maintainable setup, you can use `docker-compose`. Create a `docker-compose.yml` file with the following content:

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

Then, simply start the service by running:
```bash
docker-compose up -d
```

---

## 5. Additional Utilities

Both versions support rendering templates for testing purposes. If you want to verify what the final JSON configuration will look like after environment variables are injected, use the `render` command:

```bash
# Local Client
./portal-local render --config templates/local_config.tmpl.json --out final_config.json

# Remote Node
docker run --rm \
  --env-file .env \
  -v $(pwd):/app \
  ghcr.io/hallmaxwell/portal-svc:latest render \
    --config /app/templates/remote_config.tmpl.json \
    --out /app/final_config.json
```
