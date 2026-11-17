package swar

import (
	"encoding/binary"
	"math/bits"
)

const (
	Lanes = 0x0101010101010101
	Highs = 0x8080808080808080
	lows  = 0x7f7f7f7f7f7f7f7f
	Width = 8
)

func Load(b []byte, i int) uint64 {
	return binary.LittleEndian.Uint64(b[i:])
}

func LowerOrSpace(x uint64) (plain, spaces uint64) {
	ascii := ^x & Highs
	lower := (x + 0x1f*Lanes) &^ (x + 0x05*Lanes) & ascii
	y := x ^ ' '*Lanes
	spaces = ^((y&lows + lows) | y | lows)
	return lower | spaces, spaces
}

func PlainPrefix(x uint64) int {
	plain, _ := LowerOrSpace(x)
	return bits.TrailingZeros64(^plain&Highs) / Width
}

func Eq(x uint64, c byte) uint64 {
	y := x ^ uint64(c)*Lanes
	return ^((y&lows + lows) | y | lows)
}

func InRange(x uint64, lo, hi byte) uint64 {
	low := x & lows
	return (low + uint64(0x80-lo)*Lanes) &^ (low + uint64(0x7f-hi)*Lanes) & ^x & Highs
}

func First(mask uint64) int {
	return bits.TrailingZeros64(mask) / Width
}
