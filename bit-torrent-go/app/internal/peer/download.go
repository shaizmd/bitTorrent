package peer

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
)

func DownloadPiece(
	conn net.Conn,
	index uint32,
	pieceLength int,
	expectedHash []byte,
) ([]byte, error) {

	// 1. Wait for bitfield
	msg, err := ReadMessage(conn)
	if err != nil {
		return nil, err
	}
	if msg == nil || msg.ID != MsgBitfield {
		return nil, fmt.Errorf("expected bitfield")
	}

	_ = SendInterested(conn)

	// 2. Wait for unchoke
	for {
		msg, err := ReadMessage(conn)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}
		if msg.ID == MsgUnchoke {
			break
		}
		if msg.ID == MsgChoke {
			return nil, fmt.Errorf("choked")
		}
	}

	pieceData := make([]byte, pieceLength)

	const blockSize = 16384
	const maxPending = 5

	nextBegin := 0
	pending := 0

	// 3. Helper: send request
	sendBlock := func(begin int) error {
		length := blockSize
		if begin+length > pieceLength {
			length = pieceLength - begin
		}

		return SendRequest(conn, index, uint32(begin), uint32(length))
	}

	// 4. Prime pipeline (send first 5 requests)
	for pending < maxPending && nextBegin < pieceLength {
		if err := sendBlock(nextBegin); err != nil {
			return nil, err
		}
		nextBegin += blockSize
		pending++
	}

	// 5. Receive loop
	for pending > 0 {

		msg, err := ReadMessage(conn)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}

		if msg.ID != MsgPiece {
			continue
		}

		// Extract metadata
		begin := binary.BigEndian.Uint32(msg.Payload[4:8])
		block := msg.Payload[8:]

		copy(pieceData[begin:], block)

		pending--

		// 6. Refill pipeline
		if nextBegin < pieceLength {
			if err := sendBlock(nextBegin); err != nil {
				return nil, err
			}
			nextBegin += blockSize
			pending++
		}
	}

	// 7. Verify SHA1
	hash := sha1.Sum(pieceData)
	if !bytes.Equal(hash[:], expectedHash) {
		return nil, fmt.Errorf("hash mismatch")
	}

	return pieceData, nil
}
