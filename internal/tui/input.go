package tui

import "unicode/utf8"

// keyDecoder retains split UTF-8 characters and all keys from a pasted input batch.
type keyDecoder struct {
	tail   []byte
	keys   [][]byte
	skipLF bool
}

func (d *keyDecoder) push(chunk []byte) {
	d.tail = append(d.tail, chunk...)
	for len(d.tail) > 0 {
		if d.skipLF {
			d.skipLF = false
			if d.tail[0] == '\n' {
				d.tail = d.tail[1:]
				continue
			}
		}
		size := 1
		if d.tail[0] == 0x1b && len(d.tail) > 1 && d.tail[1] == '[' {
			if len(d.tail) < 3 {
				return
			}
			for size = 2; size < len(d.tail); size++ {
				if d.tail[size] >= 0x40 && d.tail[size] <= 0x7e {
					size++
					break
				}
			}
			if size == len(d.tail) && (d.tail[size-1] < 0x40 || d.tail[size-1] > 0x7e) {
				return
			}
		} else if d.tail[0] >= 0x80 {
			if !utf8.FullRune(d.tail) {
				return
			}
			_, size = utf8.DecodeRune(d.tail)
		}
		key := append([]byte(nil), d.tail[:size]...)
		d.keys = append(d.keys, key)
		d.skipLF = d.tail[0] == '\r'
		d.tail = d.tail[size:]
	}
}

func (d *keyDecoder) pop() []byte {
	if len(d.keys) == 0 {
		return nil
	}
	key := d.keys[0]
	d.keys[0] = nil
	d.keys = d.keys[1:]
	return key
}
