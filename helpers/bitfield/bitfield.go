package bitfield

type Bitfield []byte

func (bt Bitfield) CheckPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	return bt[byteIndex]>>(7-offset)&1 != 0
}

func (bt Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8
	bt[byteIndex] |= 1 << (7 - offset)
}
