# bit-torrent-go

A BitTorrent client implementation in Go that downloads files from torrent files by communicating with peers and tracker servers.

## Architecture Overview

The system consists of three main components:

### 1. **Bencode Package** (`internal/bencode/decoder.go`)
**Purpose**: Decodes `.torrent` files which use Bencode encoding—a binary format used by BitTorrent for metadata serialization.

**Encoding Format**:
- **Strings**: `9:hello` = "hello" (length-prefixed UTF-8)
- **Integers**: `i42e` = 42 (enclosed in `i` and `e`)
- **Lists**: `li42e4:spame` = [42, "spam"] (starts with `l`, ends with `e`)
- **Dictionaries**: `d3:agei30e4:namee5:Daniele` = {age: 30, name: "Daniel"} (starts with `d`, keys sorted lexicographically)

**How it works**: 
- Recursively parses each Bencode element starting from byte index 0
- Returns the decoded Go object (interface{}) and bytes consumed
- Handles nested structures (lists of dicts, dicts of lists, etc.)

**Example torrent file structure** (after Bencode decoding):
```
{
  "announce": "http://bittorrent-test-tracker.codecrafters.io/announce",
  "info": {
    "name": "sample.txt",
    "length": 92063,
    "piece length": 32768,
    "pieces": [<20-byte SHA-1 hashes for each piece>]
  }
}
```

### 2. **Torrent Package** (`internal/torrent/torrent.go`)
**Purpose**: Orchestrates the entire download workflow by parsing metadata and coordinating with trackers and peers.

**Key Operations**:

1. **Parse `.torrent` File**
   - Reads the binary `.torrent` file
   - Decodes using Bencode to extract all metadata
   - Validates file structure and integrity

2. **Extract Metadata**
   - **Tracker URL** (`announce`): HTTP endpoint to query for peer list
   - **File name** (`info.name`): Target filename for saved data
   - **File size** (`info.length`): Total bytes to download
   - **Piece length** (`info.piece length`): Typically 32KB per piece (must download in this unit)
   - **Piece hashes** (`info.pieces`): 20-byte SHA-1 hash for each piece (used for verification)

3. **Calculate Info Hash**
   - Takes the raw `info` dictionary bytes (before Bencode encoding is stripped)
   - Computes SHA-1 hash of these bytes
   - This **info hash** uniquely identifies the torrent and is sent to the tracker
   - Prevents spoofing: peers verify you're downloading the correct file

4. **Contact Tracker (HTTP)**
   - Sends HTTP request to announce URL with parameters:
     - `info_hash`: SHA-1 of info dictionary (URL-encoded)
     - `peer_id`: Unique identifier for this client (e.g., `-GO0001-123456789012`)
     - `port`: Listening port (typically 6881)
     - `uploaded`, `downloaded`, `left`: Progress statistics
     - `compact=1`: Request compact peer format (4 bytes IP + 2 bytes port)
   - Receives list of peers (IP:port) from tracker that have the file
   - Example response includes 20+ peers available for download

5. **Orchestrate Download**
   - Iterates through each piece sequentially
   - For each piece, connects to available peers and downloads
   - Verifies SHA-1 hash matches metadata
   - Saves piece to disk incrementally
   - Handles peer failures by retrying with different peers

### 3. **Peer Package** (`internal/peer/`)
**Purpose**: Implements the BitTorrent peer wire protocol for downloading file pieces.

**Files**:
- `download.go`: Downloads individual pieces from peers with pipelining
- `messages.go`: Defines BitTorrent message types (Choke, Unchoke, Interest, Request, Piece, etc.)

**BitTorrent Protocol Handshake**:
```
Client → Peer: 
  [19-byte length][19 bytes "BitTorrent protocol"][8 bytes flags]
  [20-byte info_hash][20-byte peer_id]

Peer → Client: 
  (same format, validates info_hash matches)
```

**Download State Machine**:
1. **Wait for Bitfield**: Peer sends bitfield indicating which pieces it has
2. **Send Interest**: Client tells peer "I'm interested in your pieces"
3. **Wait for Unchoke**: Peer responds "You may download" (or Choke = "not allowed")
4. **Send Requests**: Client sends multiple (up to 5 pipelined) `Request` messages:
   - `Request(index=0, begin=0, length=16384)` = "Send me piece 0, starting at byte 0, 16KB blocks"
5. **Receive Pieces**: Peer sends `Piece` messages with actual data
6. **Verify & Assemble**: 
   - Collect all 16KB blocks for a piece
   - Calculate SHA-1 hash of complete piece data
   - Compare against expected hash from torrent metadata
   - If mismatch: retry from different peer

**Key Protocol Details**:
- **Block Size**: 16,384 bytes (16KB) — standard BitTorrent block size
- **Pipelining**: Can have up to 5 outstanding requests simultaneously (faster than request-response)
- **Choke/Unchoke**: Peers may choke (refuse) connections if overloaded; client must handle gracefully
- **Piece Verification**: Each piece cryptographically verified using SHA-1 before saving

## Download Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User runs: go run main.go sample.torrent                    │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. ParseTorrentFile() in torrent.go                             │
│    - Read file from disk                                        │
│    - Decode Bencode → Go objects                                │
│    - Extract: announce URL, file name, size, piece hashes      │
│    - Calculate info_hash (SHA-1)                                │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Query Tracker (HTTP GET request)                             │
│    Endpoint: announce URL + params (info_hash, peer_id, etc.)   │
│    Response: List of 20+ peers (IP:port)                        │
│    Example: 165.232.38.164:51433, 165.232.35.114:51574, ...     │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. For each piece (e.g., Piece 0, Piece 1, Piece 2):            │
│                                                                  │
│    a) Connect to a peer (TCP socket)                            │
│    b) Perform BitTorrent handshake (verify info_hash)           │
│    c) Send Interest + wait for Unchoke                          │
│    d) Send 5 pipelined Requests (16KB blocks each)              │
│    e) Receive Piece messages (actual data)                      │
│    f) Calculate SHA-1 hash of complete piece                    │
│    g) Verify hash matches metadata:                             │
│       - If OK: Save to disk, move to next piece                 │
│       - If FAIL: Retry with different peer                      │
│    h) Repeat until all pieces downloaded                        │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Assemble & Save Complete File                                │
│    - Concatenate all pieces in order                            │
│    - Save as original filename (e.g., "sample.txt")             │
│    - File size = sum of all piece sizes = info.length           │
└─────────────────────────────────────────────────────────────────┘
                              ↓
                    ✓ Download Complete
```

## Example Execution

```
Tracker URL: http://bittorrent-test-tracker.codecrafters.io/announce
Name: sample.txt
Length: 92063 bytes
Info Hash: d69f91e6b2ae4c542468d1073a71d4ea13879a7f
Piece Length: 32768
Piece Hashes:
  e876f67a2a8886e8f36b136726c30fa29703022d
  6e2275e604a0766656736e81ff10b55204ad8d35
  f00d937a0213df1982bc8d097227ad9e909acc17

Contacting tracker...
Tracker Request URL: http://...&info_hash=...&peer_id=-GO0001-...

Peers discovered:
  165.232.38.164:51433
  165.232.35.114:51574
  165.232.41.73:51502

Trying to download piece 0 from 165.232.38.164:51433
Successfully downloaded piece 0: 32768 bytes

Trying to download piece 1 from 165.232.38.164:51433
Successfully downloaded piece 1: 32768 bytes

Trying to download piece 2 from 165.232.38.164:51433
Successfully downloaded piece 2: 26527 bytes

Successfully downloaded and saved 'sample.txt' (92063 bytes)
```

## Quick Start

```bash
cd "c:\Users\acer\Desktop\My Projects\GO Lang\bit-torrent-go"
go run ./app/main.go ./app/sample.torrent
```

This will automatically:
- Parse the torrent metadata
- Discover available peers from the tracker
- Download all file pieces in parallel (pipelined)
- Verify each piece using SHA-1 hashing
- Assemble and save the complete file locally

## Technical Highlights

- **Bencode Decoding**: Recursive descent parser for arbitrary nested structures
- **SHA-1 Verification**: Cryptographic validation of each piece for data integrity
- **Tracker Communication**: HTTP protocol to discover peer network
- **BitTorrent Wire Protocol**: Stateful protocol with proper handshake, interest negotiation, and pipelined requests
- **Concurrent Downloads**: Can request multiple blocks simultaneously from one peer for throughput
- **Error Handling**: Graceful handling of peer disconnects, chokes, and hash mismatches
