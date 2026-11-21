package swiftrt

import (
	"crypto/md5" //nolint:gosec // models CryptoKit Insecure.MD5
	"crypto/pbkdf2"
	"crypto/sha1" //nolint:gosec // models CryptoKit Insecure.SHA1
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"math"
)

// SHA256Digest is CryptoKit `SHA256.Digest`.
type SHA256Digest [32]byte

// SHA256HashData is CryptoKit `SHA256.hash(data:)`.
func SHA256HashData(data []byte) SHA256Digest { return sha256.Sum256(data) }

// SHA256DigestPrefixInt is `SHA256.Digest.prefix(_:)`: the first n bytes, trapping when n is negative.
func SHA256DigestPrefixInt(d SHA256Digest, n int) []uint8 {
	if n < 0 {
		Trap("Can't take a prefix of negative length from a collection")
	}
	return append([]uint8{}, d[:min(n, len(d))]...)
}

// SHA256DigestMap is `SHA256.Digest.map(_:)` over the digest's bytes.
func SHA256DigestMap[U any](d SHA256Digest, f func(uint8) U) []U {
	out := make([]U, 0, len(d))
	for _, b := range d {
		out = append(out, f(b))
	}
	return out
}

// SymmetricKey is CryptoKit `SymmetricKey`.
type SymmetricKey struct{ bytes []byte }

// SymmetricKeyWithUnsafeBytes is `SymmetricKey.withUnsafeBytes(_:)`; the raw buffer is the key's []byte.
func SymmetricKeyWithUnsafeBytes[R any](k SymmetricKey, body func(any) R) R { return body(k.bytes) }

// SliceInitUnsafeRawBufferPointer is `[UInt8](UnsafeRawBufferPointer)`: a copy of the buffer's bytes.
func SliceInitUnsafeRawBufferPointer[S ~[]uint8](p any) S {
	b := p.([]byte)
	out := make(S, len(b))
	copy(out, b)
	return out
}

// HashFunction is CryptoExtras `KDF.Insecure.PBKDF2.HashFunction`.
type HashFunction int

// The CryptoExtras PBKDF2 hash functions.
const (
	HashFunctionInsecureMD5 HashFunction = iota
	HashFunctionInsecureSHA1
	HashFunctionInsecureSHA224
	HashFunctionSha256
	HashFunctionSha384
	HashFunctionSha512
)

func (h HashFunction) new() func() hash.Hash {
	switch h {
	case HashFunctionInsecureMD5:
		return md5.New
	case HashFunctionInsecureSHA1:
		return sha1.New
	case HashFunctionInsecureSHA224:
		return sha256.New224
	case HashFunctionSha256:
		return sha256.New
	case HashFunctionSha384:
		return sha512.New384
	}
	return sha512.New
}

// PBKDF2DeriveKeyFromSaltUsingOutputByteCountUnsafeUncheckedRounds is CryptoExtras
// `KDF.Insecure.PBKDF2.deriveKey(from:salt:using:outputByteCount:unsafeUncheckedRounds:)` as built on
// BoringSSL: a negative byte count or a round count outside UInt32 traps, and zero rounds throws.
func PBKDF2DeriveKeyFromSaltUsingOutputByteCountUnsafeUncheckedRounds(password, salt []uint8, h HashFunction, outputByteCount, rounds int) SymmetricKey {
	if outputByteCount < 0 {
		Trap("Negative count")
	}
	if rounds < 0 || rounds > math.MaxUint32 {
		Trap("Not enough bits to represent the passed value")
	}
	if rounds == 0 {
		panic(Throw(NSError{Domain: "CryptoKit.CryptoKitError", Code: 0, Msg: "internalBoringSSLError"}))
	}
	if outputByteCount == 0 {
		return SymmetricKey{bytes: []byte{}}
	}
	key, err := pbkdf2.Key(h.new(), string(password), salt, rounds, outputByteCount)
	if err != nil {
		panic(Throw(NSError{Domain: "CryptoKit.CryptoKitError", Code: 0, Msg: "internalBoringSSLError: " + err.Error()}))
	}
	return SymmetricKey{bytes: key}
}
