package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

type messageID uint8

const (
	MsgChoke         messageID = 0
	MsgUnchoke       messageID = 1
	MsgInterested    messageID = 2
	MsgNotInterested messageID = 3
	MsgHave          messageID = 4
	MsgBitField      messageID = 5
	MsgRequest       messageID = 6
	MsgPiece         messageID = 7
	MsgCancel        messageID = 8
)

type Message struct {
	ID      messageID
	Payload []byte
}

func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	}
	length := uint32(len(m.Payload) + 1)
	buffer := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buffer[0:4], length)
	buffer[4] = byte(m.ID)
	copy(buffer[5:], m.Payload)
	return buffer
}

func ReadMessage(r io.Reader) (*Message, error) {
	lengthBuffer := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuffer)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuffer)

	if length == 0 {
		return nil, nil
	}

	messageBuffer := make([]byte, length)
	_, err = io.ReadFull(r, messageBuffer)
	if err != nil {
		return nil, err
	}
	m := Message{
		ID:      messageID(messageBuffer[0]),
		Payload: messageBuffer[1:],
	}
	return &m, nil
}

func ParsePieceMessage(index int, buf []byte, msg *Message) (int, error) {
	if msg.ID != MsgPiece {
		return 0, fmt.Errorf("Expected PIECE message got %x", msg.ID)
	}
	if len(msg.Payload) < 8 {
		return 0, fmt.Errorf("Expected Payload greater than 8 got %d", len(msg.Payload))
	}
	parsedIndex := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	if parsedIndex != index {
		return 0, fmt.Errorf("Got The Wrong Piece Here Expected %d", index)
	}
	begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	if begin >= len(buf) {
		return 0, fmt.Errorf("Begin is too HIGH")
	}
	data := msg.Payload[8:]
	if len(data)+begin > len(buf) {
		return 0, fmt.Errorf("Data is too long for the offset Begin %d %d", len(data), begin)
	}
	copy(buf[begin:], data)
	return len(data), nil
}

func ParseHaveMessage(msg *Message) (int, error) {
	if msg.ID != MsgHave {
		return 0, fmt.Errorf("Expected HAVE , got ID %d", msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("Expected Length got %d", msg.Payload)
	}
	index := int(binary.BigEndian.Uint32(msg.Payload))
	return index, nil
}
