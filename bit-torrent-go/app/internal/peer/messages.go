package peer

import (
    "encoding/binary"
    "io"
    "net"
)

type PeerMessage struct {
    ID      byte
    Payload []byte
}

const (
    MsgChoke       = 0
    MsgUnchoke     = 1
    MsgInterested  = 2
    MsgNotInterested = 3
    MsgHave        = 4
    MsgBitfield    = 5
    MsgRequest     = 6
    MsgPiece       = 7
    MsgCancel      = 8
)
func ReadMessage(conn net.Conn) (*PeerMessage, error) {
    // Read 4-byte length prefix
    lengthBuf := make([]byte, 4)
    _, err := io.ReadFull(conn, lengthBuf)
    if err != nil {
        return nil, err
    }

    length := binary.BigEndian.Uint32(lengthBuf)

    // Keepalive message
    if length == 0 {
        return nil, nil // Ignore keepalive
    }

    // Read message ID (1 byte)
    idBuf := make([]byte, 1)
    _, err = io.ReadFull(conn, idBuf)
    if err != nil {
        return nil, err
    }

    // Read payload (length - 1, because we already read the ID)
    payload := make([]byte, length-1)
    _, err = io.ReadFull(conn, payload)
    if err != nil {
        return nil, err
    }

    return &PeerMessage{
        ID:      idBuf[0],
        Payload: payload,
    }, nil
}
func SendInterested(conn net.Conn) error {
    interested := []byte{
        0, 0, 0, 1,  // length = 1
        2,           // id = 2 (interested)
    }
    _, err := conn.Write(interested)
    return err
}
func SendRequest(conn net.Conn, index uint32, begin uint32, length uint32) error {
    request := make([]byte, 17)
    
    // Length prefix (13 bytes for payload)
    binary.BigEndian.PutUint32(request[0:4], 13)
    
    // Message ID (6 = request)
    request[4] = 6
    
    // Piece index
    binary.BigEndian.PutUint32(request[5:9], index)
    
    // Begin offset
    binary.BigEndian.PutUint32(request[9:13], begin)
    
    // Block length
    binary.BigEndian.PutUint32(request[13:17], length)
    
    _, err := conn.Write(request)
    return err
}