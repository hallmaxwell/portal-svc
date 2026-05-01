# 📦 portal-svc

**The Ultimate Portal Template for Sing-box**

Hawego is a lightweight, efficient, and clean portal template for Sing-box, designed to provide a silky-smooth proxy experience. It consists of two main components running via a unified executable (`portal-svc`): **Transit** (Server) and **Dock** (Client).

---

## 📂 Project Structure

```text
.
├── cmd/
│   └── svc/         # Unified service main entrypoint
│       ├── main.go
│       └── main_test.go
├── transit/         # Server-side deployment files
│   ├── Dockerfile
│   └── docker-compose.yml
├── util/            # Shared utilities
├── dock_config.tmpl.json      # Sing-box client configuration template
├── transit_config.tmpl.json   # Sing-box server configuration template
├── Dockerfile                 # Unified Docker image definition
└── README.md        # You are here
```

---

## 🚀 Transit Node (Server)

The Transit node acts as a secure relay. It accepts incoming **VLESS + REALITY** connections and forwards the traffic to a specified **SOCKS5** outbound.

### 🐳 Deployment

The Transit node is designed to be deployed using Docker.

1.  Navigate to the transit directory: `cd transit`
2.  Create a `.env` file with the required parameters (see below).
3.  Deploy using Docker Compose:
    ```bash
    docker-compose up -d
    ```

### ⚙️ Runtime Parameters (.env)

| Variable | Description |
| :--- | :--- |
| `UUID` | VLESS User UUID |
| `PRIVATE_KEY` | REALITY Private Key |
| `SHORT_ID` | REALITY Short ID |
| `PROXY_IP` | Upstream SOCKS5 Proxy IP |
| `PROXY_PORT` | Upstream SOCKS5 Proxy Port |
| `PROXY_USERNAME` | Upstream SOCKS5 Proxy Username |
| `PROXY_PASSWORD` | Upstream SOCKS5 Proxy Password |

---

## ⚓ Dock Node (Client)

The Dock node is a client-side wrapper for Sing-box. It sets up a **TUN interface** for transparent proxying and connects to the Transit node. It is designed to run as a background service using the `dock` subcommand of the unified executable.

### 🛠️ Deployment

1.  Build the unified executable (for Windows, be sure to set `GOOS=windows` as it uses Windows-specific syscalls like `HideWindow`):
    ```bash
    go build -o portal-svc ./cmd/svc/main.go
    ```
2.  Create a `.env` file in the same directory as the executable.
3.  Ensure the `sing-box` binary is available in a `core/` subdirectory or in your system path.
4.  Manage the service (requires administrative/root privileges):
    *   **Install**: `./portal-svc dock install`
    *   **Start**: `./portal-svc dock start`
    *   **Stop**: `./portal-svc dock stop`
    *   **Restart**: `./portal-svc dock restart`
    *   **Uninstall**: `./portal-svc dock uninstall`
    *   **View Logs**: `./portal-svc dock logs [-f] [-n 100]`

### ⚙️ Runtime Parameters (.env)

| Variable | Description |
| :--- | :--- |
| `DO_IP` | IP address of the Transit Node |
| `UUID` | VLESS User UUID (must match Transit) |
| `PUBLIC_KEY` | REALITY Public Key (matching Transit's Private Key) |
| `SHORT_ID` | REALITY Short ID |
| `BYPASS_DOMAINS` | JSON array of domains to bypass (e.g., `["example.com", "google.cn"]`) |

---

## 📖 Usage Workflow

1.  **Setup Transit**: Deploy the Transit node on your server with your upstream SOCKS5 proxy details.
2.  **Generate Keys**: Generate a UUID and a X25519 key pair for VLESS+REALITY.
3.  **Configure Dock**: Fill in the `.env` on your client machine with the Transit server's IP and the generated keys.
4.  **Run Dock**: Install and start the Dock service to begin proxying your system traffic through the TUN interface.

---

## 🛠️ Utilities

*   **Core Portal**: Guided logic for project initialization.
*   **Sing-box Engine**: Deeply integrated core forwarding engine.
*   **Auto Workflow**: CI/CD integration for automated builds.
*   **Security Layer**: Custom security protection strategies.
