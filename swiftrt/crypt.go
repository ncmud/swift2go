package swiftrt

import "strings"

// cStringBytes is the bytes of a C string argument before its NUL, undecoded.
func cStringBytes(p *any) string {
	if p == nil {
		Trap("unexpectedly found nil while unwrapping an Optional value")
	}
	var b []byte
	for i := 0; ; i++ {
		c := cCharAt(*p, i)
		if c == 0 {
			return string(b)
		}
		b = append(b, byte(c))
	}
}

// CFCrypt is C `crypt(_:_:)` as libxcrypt implements it on Linux, for the traditional DES hash:
// a setting starting with two characters from `./0-9A-Za-z`, such as a stored hash, gives the
// 13-character hash of the key's first eight characters. A setting that is not a DES salt, or that
// contains a space, control or non-ASCII byte or one of `!*:;\`, gives the failure token "*0", or
// "*1" when the setting itself starts with "*0". Settings naming another hash method (`$id$…`,
// `_…`) panic as unsupported.
func CFCrypt(key, setting *any) *any {
	k, s := cStringBytes(key), cStringBytes(setting)
	valid := len(s) >= 2
	for i := 0; i < len(s) && valid; i++ {
		valid = s[i] > ' ' && s[i] < 0x7f && strings.IndexByte(`!*:;\`, s[i]) < 0
	}
	if valid && (s[0] == '$' || s[0] == '_') {
		panic("swiftrt: crypt hash method not supported: " + s)
	}
	if !valid || cryptIndex(s[0]) < 0 || cryptIndex(s[1]) < 0 {
		if strings.HasPrefix(s, "*0") {
			return cStringResult("*1")
		}
		return cStringResult("*0")
	}
	return cStringResult(desCrypt(k, s[:2]))
}

const cryptAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func cryptIndex(c byte) int { return strings.IndexByte(cryptAlphabet, c) }

var (
	desIP = [64]byte{58, 50, 42, 34, 26, 18, 10, 2, 60, 52, 44, 36, 28, 20, 12, 4,
		62, 54, 46, 38, 30, 22, 14, 6, 64, 56, 48, 40, 32, 24, 16, 8,
		57, 49, 41, 33, 25, 17, 9, 1, 59, 51, 43, 35, 27, 19, 11, 3,
		61, 53, 45, 37, 29, 21, 13, 5, 63, 55, 47, 39, 31, 23, 15, 7}
	desFP = [64]byte{40, 8, 48, 16, 56, 24, 64, 32, 39, 7, 47, 15, 55, 23, 63, 31,
		38, 6, 46, 14, 54, 22, 62, 30, 37, 5, 45, 13, 53, 21, 61, 29,
		36, 4, 44, 12, 52, 20, 60, 28, 35, 3, 43, 11, 51, 19, 59, 27,
		34, 2, 42, 10, 50, 18, 58, 26, 33, 1, 41, 9, 49, 17, 57, 25}
	desPC1C = [28]byte{57, 49, 41, 33, 25, 17, 9, 1, 58, 50, 42, 34, 26, 18,
		10, 2, 59, 51, 43, 35, 27, 19, 11, 3, 60, 52, 44, 36}
	desPC1D = [28]byte{63, 55, 47, 39, 31, 23, 15, 7, 62, 54, 46, 38, 30, 22,
		14, 6, 61, 53, 45, 37, 29, 21, 13, 5, 28, 20, 12, 4}
	desShifts = [16]byte{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
	desPC2C   = [24]byte{14, 17, 11, 24, 1, 5, 3, 28, 15, 6, 21, 10,
		23, 19, 12, 4, 26, 8, 16, 7, 27, 20, 13, 2}
	desPC2D = [24]byte{41, 52, 31, 37, 47, 55, 30, 40, 51, 45, 33, 48,
		44, 49, 39, 56, 34, 53, 46, 42, 50, 36, 29, 32}
	desE = [48]byte{32, 1, 2, 3, 4, 5, 4, 5, 6, 7, 8, 9,
		8, 9, 10, 11, 12, 13, 12, 13, 14, 15, 16, 17,
		16, 17, 18, 19, 20, 21, 20, 21, 22, 23, 24, 25,
		24, 25, 26, 27, 28, 29, 28, 29, 30, 31, 32, 1}
	desS = [8][64]byte{
		{14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7,
			0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8,
			4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0,
			15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13},
		{15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10,
			3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10, 6, 9, 11, 5,
			0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15,
			13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9},
		{10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8,
			13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1,
			13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7,
			1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12},
		{7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15,
			13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9,
			10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4,
			3, 15, 0, 6, 10, 1, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14},
		{2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9,
			14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6,
			4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14,
			11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3},
		{12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11,
			10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8,
			9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6,
			4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13},
		{4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1,
			13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6,
			1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2,
			6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12},
		{13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7,
			1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2,
			7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8,
			2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11},
	}
	desP = [32]byte{16, 7, 20, 21, 29, 12, 28, 17, 1, 15, 23, 26, 5, 18, 31, 10,
		2, 8, 24, 14, 32, 27, 3, 9, 19, 13, 30, 6, 22, 11, 4, 25}
)

// desCrypt is the traditional Unix DES crypt: 25 encryptions of a zero block under the key, with
// the salt swapping pairs of expansion bits.
func desCrypt(key, salt string) string {
	var keyBits [64]byte
	for i := 0; i < 8 && i < len(key) && key[i] != 0; i++ {
		for j := range 7 {
			keyBits[8*i+j] = (key[i] >> (6 - j)) & 1
		}
	}
	var c, d [28]byte
	for i := range 28 {
		c[i], d[i] = keyBits[desPC1C[i]-1], keyBits[desPC1D[i]-1]
	}
	var ks [16][48]byte
	for i := range 16 {
		for range desShifts[i] {
			c = [28]byte(append(c[1:], c[0]))
			d = [28]byte(append(d[1:], d[0]))
		}
		for j := range 24 {
			ks[i][j] = c[desPC2C[j]-1]
			ks[i][j+24] = d[desPC2D[j]-28-1]
		}
	}
	e := desE
	for i := range 2 {
		v := cryptIndex(salt[i])
		for j := range 6 {
			if v>>j&1 != 0 {
				e[6*i+j], e[6*i+j+24] = e[6*i+j+24], e[6*i+j]
			}
		}
	}
	var block [66]byte
	for range 25 {
		var lr [64]byte
		for j := range 64 {
			lr[j] = block[desIP[j]-1]
		}
		l, r := [32]byte(lr[:32]), [32]byte(lr[32:])
		for i := range 16 {
			var f [32]byte
			for k := range 8 {
				var x [6]byte
				for t := range 6 {
					x[t] = r[e[6*k+t]-1] ^ ks[i][6*k+t]
				}
				s := desS[k][x[0]<<5|x[5]<<4|x[1]<<3|x[2]<<2|x[3]<<1|x[4]]
				for t := range 4 {
					f[4*k+t] = s >> (3 - t) & 1
				}
			}
			next := l
			for j := range 32 {
				next[j] ^= f[desP[j]-1]
			}
			l, r = r, next
		}
		copy(lr[:32], r[:])
		copy(lr[32:], l[:])
		for j := range 64 {
			block[j] = lr[desFP[j]-1]
		}
	}
	out := []byte(salt)
	for i := range 11 {
		v := 0
		for j := range 6 {
			v = v<<1 | int(block[6*i+j])
		}
		out = append(out, cryptAlphabet[v])
	}
	return string(out)
}
