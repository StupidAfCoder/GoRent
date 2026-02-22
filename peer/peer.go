package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"bitTorrent/helpers/bitfield"
	"bitTorrent/message"
)

type Peer struct {
	IP   net.IP
	port uint16
}

func (p Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(int(p.port)))
}

func Unmarshal(peersBin []byte) ([]Peer, error) {
	const peerSize = 6
	numPeers := len(peersBin) / peerSize
	if len(peersBin)%peerSize != 0 {
		err := fmt.Errorf("MalInformation recieved")
		return nil, err
	}
	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		peers[i].IP = net.IP(peersBin[offset : offset+4])
		peers[i].port = binary.BigEndian.Uint16(peersBin[offset+4 : offset+6])
	}
	return peers, nil
}

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

func New(infohash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     "BitTorrent protocol",
		InfoHash: infohash,
		PeerID:   peerID,
	}
}

func (h *Handshake) Serialize() []byte {
	buffer := make([]byte, len(h.Pstr)+49)
	cursor := 1
	buffer[0] = byte(len(h.Pstr))
	cursor += copy(buffer[cursor:], h.Pstr)
	cursor += copy(buffer[cursor:], make([]byte, 8))
	cursor += copy(buffer[cursor:], h.InfoHash[:])
	cursor += copy(buffer[cursor:], h.PeerID[:])
	return buffer
}

func ReadHandShake(r io.Reader) (*Handshake, error) {
	lengthBuffer := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuffer)
	if err != nil {
		return nil, err
	}
	pstrlen := int(lengthBuffer[0])
	handshakeBuffer := make([]byte, pstrlen+48)
	_, err = io.ReadFull(r, handshakeBuffer)
	if err != nil {
		return nil, err
	}
	h := Handshake{}
	h.Pstr = string(handshakeBuffer[0:pstrlen])
	cursor := pstrlen
	cursor += 8
	copy(h.InfoHash[:], handshakeBuffer[cursor:cursor+20])
	cursor += 20
	copy(h.PeerID[:], handshakeBuffer[cursor:cursor+20])
	return &h, nil
}

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield bitfield.Bitfield
	peer     Peer
	peerID   [20]byte
	infoHash [20]byte
}

func (c *Client) Read() (*message.Message, error) {
	msg, err := message.ReadMessage(c.Conn)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (c *Client) SendRequest(index, begin, length int) error {
	req := formatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SendInterested() error {
	msg := message.Message{ID: message.MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SendNotInterested() error {
	msg := message.Message{ID: message.MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SendUnchoke() error {
	msg := message.Message{ID: message.MsgUnchoke}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) SendHave(index int) error {
	msg := formatHave(index)
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func formatHave(index int) *message.Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(index))
	return &message.Message{ID: message.MsgHave, Payload: payload}
}

func formatRequest(index, begin, length int) *message.Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return &message.Message{ID: message.MsgRequest, Payload: payload}
}

func completeHandshake(conn net.Conn, peerid [20]byte, infohash [20]byte) (*Handshake, error) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	request := New(infohash, peerid)
	_, err := conn.Write(request.Serialize())
	if err != nil {
		return nil, err
	}

	response, err := ReadHandShake(conn)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(response.InfoHash[:], infohash[:]) {
		return nil, fmt.Errorf("expected infohash %x but got %x", response.InfoHash, infohash)
	}

	return response, nil
}

func recieveBitField(conn net.Conn) (bitfield.Bitfield, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{})

	msg, err := message.ReadMessage(conn)
	if err != nil {
		return nil, err
	}
	if msg.ID != message.MsgBitField {
		err := fmt.Errorf("expected bitfield but got Id %d", msg.ID)
		return nil, err
	}

	return msg.Payload, nil
}

func NewClient(peer Peer, peerid [20]byte, infohash [20]byte) (*Client, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = completeHandshake(conn, peerid, infohash)
	if err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := recieveBitField(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bf,
		peer:     peer,
		peerID:   peerid,
		infoHash: infohash,
	}, nil
}
