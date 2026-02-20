# GreenCloud FileServer (Hedged Read Server)

A high-performance, lightweight Go-based file server engineered specifically to combat long-tail I/O latency (Tail Latency) and cache stampedes in storage environments like HDD-based VPS, NAS, or network mounts. **This project was specifically designed to handle the notoriously degraded and volatile HDD I/O performance often experienced on GreenCloud VPS's budget storage plans.**

Designed for streaming large media files (e.g., HLS/DASH `.m4s` segments) where consistent playback performance is critical.

## 🌟 Key Features

### 1. Hedged Requests (快速熔断重试)
Under normal circumstances, if a slow disk sector or I/O queue spike stalls a `read()` syscall, the stream simply hangs. This server employs a **Hedged Read** strategy:
- It actively measures the I/O speed.
- If the speed drops below a configurable threshold (e.g., `< 5Mbps`) within the first second, the read is forcefully aborted.
- A micro-pause (e.g., 100ms) occurs, allowing the kernel to potentially load data into the Page Cache.
- A secondary retry is executed. In many cases, the second request hits the kernel cache, entirely skipping the physical disk read bottleneck.

### 2. Singleflight Anti-Stampede (防并发击穿)
Multiple users requesting the exact same file (e.g., users in a Synctube room watching the same video) won't thrash the disk. Using `golang.org/x/sync/singleflight`, all concurrent requests for the same path are collapsed into **one** underlying disk read.
- **Context Detachment Safety (The Secret Sauce):** The disk read lifecycle is detached from the original HTTP Request context. If the initiating user abruptly disconnects or seeks, the file is still fully read into memory for the *other* waiting users, preventing a cascading failure.

### 3. Native Range Request Support (206 Partial Content)
The files cached in memory are seamlessly bridged to standard `http.ServeContent`. This means seeking forward/backward over a video natively utilizes `Range` HTTP requests. Only a single full disk read is ever performed; subsequent slice retrievals are instantly served from the RAM cache.

### 4. Application-Layer LRU Cache
Since standard Nginx configurations limit cache manipulation capabilities, we bring it directly into the application space.
- Configurable maximum size limit (e.g., `1GB`).
- Doubly-linked list LRU eviction ensures active media segments stay hot while old tracks are pruned.

## 🚀 Deployment (Docker Compose)

The easiest way to run the GreenCloud FileServer is via the pre-built Docker image. Below is a sample `docker-compose.yml` demonstrating how to mount your raw disk media and map the port.

```yaml
version: '3'
services:
  media-server:
    image: ghcr.io/t0saki/greencloud-fileserver:latest
    container_name: go-media-server
    restart: always
    environment:
      # Memory Cache Size limit (in bytes) - Example: 1GB = 1073741824
      - CACHE_SIZE_BYTES=1073741824 
      # Mount point inside container (defaults to /data)
      - SERVE_DIR=/data
    volumes:
      # Change /root/docker/cinema/rawdata to the path containing your files
      - /root/docker/cinema/rawdata:/data:ro
    ports:
      # Map container port 8080 to host's desired port
      - "172.17.0.1:21545:8080"
```

### Advanced Environment Variables

- `CACHE_SIZE_BYTES` - Maximum allocation bounds for the memory cache. (Default: 1GB)
- `SERVE_DIR` - Which directory to serve from. (Default: `/data`)
- `PORT` - The internal port to expose. (Default: `8080`)

*(Additionally, properties such as time to check, min-speed Mbps, and hedged-delay are available via CLI flags).*

## 🛠 Building from Source

```bash
# Clone the repository
git clone https://github.com/t0saki/GreenCloud-FileServer.git
cd GreenCloud-FileServer

# Build locally
go build -o fileserver .

# Run manually
./fileserver -dir ./mydata -port 8080
```

## 🤝 Architecture Inspiration

This design was tailored particularly to circumvent issues when traditional reverse proxies like Nginx use generic buffer techniques over low-tier standard block storage. By migrating the buffer strategy to User Space and maintaining tight `read()` telemetry, it ensures the best possible application-layer QoS.
