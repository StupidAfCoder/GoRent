# 🧲 GoRent — BitTorrent Client in Go

> A fully working BitTorrent client built from scratch in Go. Connects to HTTP trackers, discovers peers, and downloads files concurrently using the BitTorrent wire protocol.

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey?style=flat)
![License](https://img.shields.io/badge/license-MIT-green?style=flat)
![Status](https://img.shields.io/badge/status-working-brightgreen?style=flat)

---

## ✨ Features

- **Full BitTorrent wire protocol** — handshake, bitfield, choke/unchoke, piece requests
- **Concurrent downloads** — one goroutine per peer, fully pipelined block requests
- **SHA-1 integrity verification** — every piece is verified before being written to disk
- **Exponential back-off reconnection** — dropped peers are retried automatically
- **HTTP/HTTPS tracker support** — announces, compact peer lists
- **Flexible input** — pass a `.torrent` file as an argument or pipe it via stdin
- **Cross-platform** — runs on Linux, macOS, and Windows with no code changes

---

## 📁 Project Structure

```
GoRent/
├── main.go                         # Entry point — CLI args, stdin, orchestration
├── go.mod / go.sum                 # Module definition and dependency lock
│
├── message/
│   └── message.go                  # Wire protocol — Message type, ReadMessage, Serialize
│
├── peer/
│   └── peer.go                     # Peer struct, Client, handshake, send/receive helpers
│
├── torrent/
│   └── torrent.go                  # .torrent parsing, tracker requests, download engine
│
├── helpers/
│   └── bitfield/
│       └── bitfield.go             # Bitmap for tracking which pieces each peer holds
│
└── test/
    └── debian-13.3.0-amd64-netinst.iso.torrent   # Ready-to-use test torrent
```

---

## 🔧 Requirements

| Requirement | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.21 or later |
| Internet connection | Required for tracker + peer communication |
| Firewall | Allow outbound TCP (common ports: 6881, 6882) |

---

## 🚀 Installation

```bash
git clone https://github.com/StupidAfCoder/GoRent.git
cd GoRent
go mod tidy
```

---

## ▶️ Usage

### Linux / macOS

**Pass a `.torrent` file as an argument (recommended):**
```bash
go build -o gorent .
./gorent path/to/file.torrent
```
### Show verbose debug output
```bash
./gorent -v file.torrent
```
By default, peer noise (handshake failures, disconnects) is hidden.
Use `-v` if a download is stalling and you want to see what's going wrong.

**Pipe via stdin:**
```bash
cat path/to/file.torrent | ./gorent
```

**Quick test with the included Debian torrent:**
```bash
./gorent test/debian-13.3.0-amd64-netinst.iso.torrent
```

---

### 🪟 Windows

GoRent works on Windows with no code changes. The only differences are shell syntax.

**Build:**
```powershell
go build -o gorent.exe .
```

**Run (Command Prompt):**
```cmd
gorent.exe path\to\file.torrent
```

**Run (PowerShell):**
```powershell
.\gorent.exe path\to\file.torrent
```

**Pipe via stdin (PowerShell):**
```powershell
Get-Content -Raw path\to\file.torrent | .\gorent.exe
```

**Pipe via stdin (Command Prompt):**
```cmd
type path\to\file.torrent | gorent.exe
```

**Quick test (PowerShell):**
```powershell
.\gorent.exe test\debian-13.3.0-amd64-netinst.iso.torrent
```

---

## 📊 Example Output

```
2025/01/15 14:23:01 Starting Download For debian-13.3.0-amd64-netinst.iso
Number Of Peers 42
(0.21%) Downloaded Piece 87 from 38 peers
(0.43%) Downloaded Piece 12 from 40 peers
(0.64%) Downloaded Piece 205 from 41 peers
...
(100.00%) Downloaded Piece 991 from 35 peers
The Torrent Has Been Saved To Your Computer --> debian-13.3.0-amd64-netinst.iso
```

---

## ⚠️ Limitations

| Limitation | Details |
|---|---|
| **UDP trackers not supported** | Only `http://` and `https://` tracker URLs are accepted. Torrents whose announce URL uses `udp://` will exit with an error. |
| **No magnet link support** | A `.torrent` file is required. Magnet links are not parsed. |
| **No resume / partial downloads** | Each run starts from scratch. If interrupted, the download begins again. |
| **Single-file torrents only** | Multi-file `.torrent` bundles are not yet supported. |
| **No seeding** | GoRent is a download-only client. It does not seed back to the swarm after completion. |

---

## 🔬 Technical Details

| Constant | Value | Reason |
|---|---|---|
| `BLOCKSIZE` | 16,384 bytes (16 KiB) | Maximum allowed by the BitTorrent spec |
| `MAXBACKLOG` | 100 requests | Pipelined in-flight requests per peer — higher = faster on high-latency connections |
| Handshake timeout | 3 seconds | Per-peer connection deadline |
| Bitfield timeout | 5 seconds | Time to receive the peer's bitfield after handshake |
| Piece timeout | 30 seconds | Per-piece download deadline before abandoning a peer |
| Reconnect back-off | 1s → 2s → 4s … 30s max | Exponential back-off on failed peer connections |

---

## 📦 Dependencies

| Package | Purpose |
|---|---|
| [`github.com/jackpal/bencode-go`](https://github.com/jackpal/bencode-go) | Bencode encoding/decoding for `.torrent` files and tracker responses |

---

## 📄 License

MIT — see [LICENSE](LICENSE) for details.
