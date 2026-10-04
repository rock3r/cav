package fontmeta

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func fixtureFont() []byte {
	records := []struct {
		id   uint16
		text string
	}{{1, "Wrong Legacy"}, {16, "Fixture Variable"}, {256, "Weight"}, {257, "Roundness"}}
	name := make([]byte, 6+len(records)*12)
	order.PutUint16(name[2:], uint16(len(records)))
	order.PutUint16(name[4:], uint16(len(name)))
	strings := []byte{}
	for i, r := range records {
		record := name[6+i*12:]
		order.PutUint16(record, 3)
		order.PutUint16(record[2:], 1)
		order.PutUint16(record[4:], 0x409)
		order.PutUint16(record[6:], r.id)
		start := len(strings)
		for _, c := range utf16.Encode([]rune(r.text)) {
			strings = binary.BigEndian.AppendUint16(strings, c)
		}
		order.PutUint16(record[8:], uint16(len(strings)-start))
		order.PutUint16(record[10:], uint16(start))
	}
	name = append(name, strings...)
	fvar := make([]byte, 56)
	order.PutUint16(fvar, 1)
	order.PutUint16(fvar[4:], 16)
	order.PutUint16(fvar[8:], 2)
	order.PutUint16(fvar[10:], 20)
	for i, tag := range []string{"wght", "ROND"} {
		r := fvar[16+i*20:]
		copy(r, tag)
		order.PutUint32(r[8:], 400<<16)
		order.PutUint32(r[12:], 1000<<16)
		order.PutUint16(r[18:], uint16(256+i))
	}
	b := make([]byte, 44)
	copy(b, []byte{0, 1, 0, 0})
	order.PutUint16(b[4:], 2)
	copy(b[12:], "fvar")
	order.PutUint32(b[20:], 44)
	order.PutUint32(b[24:], uint32(len(fvar)))
	copy(b[28:], "name")
	order.PutUint32(b[36:], 44+uint32(len(fvar)))
	order.PutUint32(b[40:], uint32(len(name)))
	return append(append(b, fvar...), name...)
}
func TestFontAxesUseFvarOrderAndBounds(t *testing.T) {
	b := fixtureFont()
	font, ok := readFont(bytes.NewReader(b), 0)
	if !ok {
		t.Fatal("valid fvar rejected")
	}
	if font.Family != "Fixture Variable" || font.Axes[1].Tag != "ROND" || font.Axes[0].Default != 400 || font.Axes[0].Max != 1000 {
		t.Fatalf("wrong metadata: %+v", font)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.ttf"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if len(Discover([]string{dir})[font.Family]) != 2 {
		t.Fatal("font discovery failed")
	}
	for _, n := range []int{0, 12, 30, 45, 80} {
		if _, ok := readFont(bytes.NewReader(b[:n]), 0); ok {
			t.Fatalf("truncated file %d accepted", n)
		}
	}
	order.PutUint16(b[44+10:], 1)
	if _, ok := readFont(bytes.NewReader(b), 0); ok {
		t.Fatal("invalid axis size accepted")
	}
}
func FuzzFontTables(f *testing.F) {
	f.Add(fixtureFont())
	f.Add([]byte("bad font"))
	f.Fuzz(func(t *testing.T, b []byte) { readFont(bytes.NewReader(b), 0) })
}
