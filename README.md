# GoRent — BitTorrent Client in Go

A BitTorrent client built from scratch in Go. Connects to HTTP trackers, discovers peers, and downloads files concurrently using the BitTorrent wire protocol.

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey?style=flat)
![License](https://img.shields.io/badge/license-MIT-green?style=flat)

---



https://github.com/user-attachments/assets/a46c96d0-8e81-47db-8cab-feb9f005d4f5



---

## Project Structure

```
GoRent/
├── main.go
├── go.mod / go.sum
├── message/
│   └── message.go          # Wire protocol — Message type, serialization, parsing
├── peer/
│   └── peer.go             # Peer struct, Client, handshake, send/receive helpers
├── torrent/
│   └── torrent.go          # .torrent parsing, tracker requests, download engine
├── helpers/
│   └── bitfield/
│       └── bitfield.go     # Bitmap for tracking which pieces each peer has
└── test/
    └── debian-13.3.0-amd64-netinst.iso.torrent
```

---

## Requirements

- Go 1.21 or later
- Internet connection (tracker + peer communication)
- Outbound TCP allowed on your firewall (port 6881)

---

## Installation

```bash
git clone https://github.com/StupidAfCoder/GoRent.git
cd GoRent
go mod tidy
```

---

## Usage

**Build:**
```bash
go build -o gorent .
```

**Run with a .torrent file:**
```bash
./gorent path/to/file.torrent
```

**Verbose mode** (shows handshake failures, disconnects, peer noise):
```bash
./gorent -v path/to/file.torrent
```

**Pipe via stdin:**
```bash
cat path/to/file.torrent | ./gorent
```

**Quick test with the included Debian torrent:**
```bash
./gorent test/debian-13.3.0-amd64-netinst.iso.torrent
```

### Windows

```powershell
go build -o gorent.exe .
.\gorent.exe path\to\file.torrent

# Pipe via stdin
Get-Content -Raw path\to\file.torrent | .\gorent.exe
```

---

## Example Output

```
2025/01/15 14:23:01 Starting Download For debian-13.3.0-amd64-netinst.iso
Number Of Peers 42
(0.21%) Downloaded Piece 87 from 38 peers
(0.43%) Downloaded Piece 12 from 40 peers
...
(100.00%) Downloaded Piece 991 from 35 peers
The Torrent Has Been Saved To Your Computer --> debian-13.3.0-amd64-netinst.iso
```

---

## Technical Details

| Constant | Value | Reason |
|---|---|---|
| `BLOCKSIZE` | 16,384 bytes | Maximum block size per BitTorrent spec |
| `MAXBACKLOG` | 100 requests | In-flight pipelined requests per peer |
| Handshake timeout | 3 seconds | Per-peer connection deadline |
| Bitfield timeout | 5 seconds | Time to receive bitfield after handshake |
| Piece timeout | 30 seconds | Per-piece deadline before dropping a peer |
| Reconnect backoff | 1s → 2s → 4s … 30s max | Exponential backoff on failed connections |

---

## Limitations

- **UDP trackers not supported.** Only `http://` and `https://` announce URLs work. If a torrent's tracker uses `udp://`, the client exits with an error.
- **No magnet link support.** A `.torrent` file is required.
- **No resume.** If a download is interrupted, it starts over from scratch.
- **Single-file torrents only.** Multi-file `.torrent` bundles are not supported.
- **No seeding.** GoRent downloads only. It does not upload back to the swarm.

---

## Dependencies

- [`github.com/jackpal/bencode-go`](https://github.com/jackpal/bencode-go) — bencode encoding/decoding for `.torrent` files and tracker responses

---

## License

MIT — see [LICENSE](LICENSE) for details.
