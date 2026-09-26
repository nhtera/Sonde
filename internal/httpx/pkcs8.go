// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des" //nolint:gosec // G502: legacy keys may use 3DES; only used to decrypt
	"crypto/pbkdf2"
	"crypto/sha1" //nolint:gosec // G505: PBKDF2 PRF of legacy keys
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"hash"
)

// ASN.1 structures of an encrypted PKCS #8 private key (RFC 5958, PBES2
// from RFC 8018).
type encryptedPrivateKeyInfo struct {
	Algorithm     algorithmIdentifier
	EncryptedData []byte
}

type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type pbes2Params struct {
	KeyDerivation algorithmIdentifier
	Encryption    algorithmIdentifier
}

type pbkdf2Params struct {
	Salt       []byte
	Iterations int
	KeyLength  int                 `asn1:"optional"`
	PRF        algorithmIdentifier `asn1:"optional"`
}

var (
	oidPBES2      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}
	oidAES128CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidDESEDE3CBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
)

// maxPBKDF2Iterations bounds the work a key file can ask for; common
// tools use 2048 to a few hundred thousand.
const maxPBKDF2Iterations = 10_000_000

// decryptKeyPEM returns keyPEM with an "ENCRYPTED PRIVATE KEY" block
// decrypted with password into an unencrypted "PRIVATE KEY" block; other
// input is returned unchanged.
func decryptKeyPEM(keyPEM []byte, password string) ([]byte, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "ENCRYPTED PRIVATE KEY" {
		return keyPEM, nil
	}
	der, err := decryptPKCS8(block.Bytes, []byte(password))
	if err != nil {
		return nil, err
	}
	if _, err := x509.ParsePKCS8PrivateKey(der); err != nil {
		return nil, errors.New("wrong password or unsupported key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func decryptPKCS8(der, password []byte) ([]byte, error) {
	var info encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		return nil, err
	}
	if !info.Algorithm.Algorithm.Equal(oidPBES2) {
		return nil, errors.New("unsupported key encryption (only PBES2)")
	}
	var params pbes2Params
	if _, err := asn1.Unmarshal(info.Algorithm.Parameters.FullBytes, &params); err != nil {
		return nil, err
	}
	if !params.KeyDerivation.Algorithm.Equal(oidPBKDF2) {
		return nil, errors.New("unsupported key derivation (only PBKDF2)")
	}
	var kdf pbkdf2Params
	if _, err := asn1.Unmarshal(params.KeyDerivation.Parameters.FullBytes, &kdf); err != nil {
		return nil, err
	}
	if kdf.Iterations < 1 || kdf.Iterations > maxPBKDF2Iterations {
		return nil, errors.New("unsupported PBKDF2 iteration count")
	}
	prf := func() hash.Hash { return sha1.New() } //nolint:gosec // G401: default PRF of PBKDF2
	switch {
	case kdf.PRF.Algorithm == nil, kdf.PRF.Algorithm.Equal(oidHMACSHA1):
	case kdf.PRF.Algorithm.Equal(oidHMACSHA256):
		prf = sha256.New
	case kdf.PRF.Algorithm.Equal(oidHMACSHA512):
		prf = sha512.New
	default:
		return nil, errors.New("unsupported PBKDF2 function")
	}
	var iv []byte
	if _, err := asn1.Unmarshal(params.Encryption.Parameters.FullBytes, &iv); err != nil {
		return nil, err
	}
	var keyLen int
	var newCipher func([]byte) (cipher.Block, error)
	switch enc := params.Encryption.Algorithm; {
	case enc.Equal(oidAES128CBC):
		keyLen, newCipher = 16, aes.NewCipher
	case enc.Equal(oidAES192CBC):
		keyLen, newCipher = 24, aes.NewCipher
	case enc.Equal(oidAES256CBC):
		keyLen, newCipher = 32, aes.NewCipher
	case enc.Equal(oidDESEDE3CBC):
		keyLen, newCipher = 24, des.NewTripleDESCipher
	default:
		return nil, errors.New("unsupported key cipher")
	}
	if kdf.KeyLength != 0 && kdf.KeyLength != keyLen {
		return nil, errors.New("invalid PBKDF2 key length")
	}
	key, err := pbkdf2.Key(prf, string(password), kdf.Salt, kdf.Iterations, keyLen)
	if err != nil {
		return nil, err
	}
	c, err := newCipher(key)
	if err != nil {
		return nil, err
	}
	data := info.EncryptedData
	if len(iv) != c.BlockSize() || len(data) == 0 || len(data)%c.BlockSize() != 0 {
		return nil, errors.New("invalid encrypted key")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(c, iv).CryptBlocks(out, data)
	last := out[len(out)-1]
	pad := int(last)
	if pad == 0 || pad > c.BlockSize() || !bytes.Equal(out[len(out)-pad:], bytes.Repeat([]byte{last}, pad)) {
		return nil, errors.New("wrong password")
	}
	return out[:len(out)-pad], nil
}
