package gatesentry2storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"

	"io"
	"strings"
)

const (
	gcmEnvelopeVersion = "aes-256-gcm-v1"
	gcmPayloadPrefix   = "gcm1:"
)

func gcmAdditionalData(storeName string) []byte {
	return []byte("gatesentry-storage:" + gcmEnvelopeVersion + "\x00" + storeName)
}

func addBase64Padding(value string) string {
	m := len(value) % 4
	if m != 0 {
		value += strings.Repeat("=", 4-m)
	}

	return value
}

func removeBase64Padding(value string) string {
	return strings.Replace(value, "=", "", -1)
}

func Pad(src []byte) []byte {
	padding := aes.BlockSize - len(src)%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func Unpad(src []byte) ([]byte, error) {
	length := len(src)
	if length == 0 {
		return nil, errors.New("unpad error: empty plaintext")
	}
	unpadding := int(src[length-1])

	if unpadding == 0 || unpadding > aes.BlockSize || unpadding > length {
		return nil, errors.New("unpad error. This could happen when incorrect encryption key is used")
	}
	for _, value := range src[length-unpadding:] {
		if int(value) != unpadding {
			return nil, errors.New("unpad error: invalid padding bytes")
		}
	}

	return src[:(length - unpadding)], nil
}

func encrypt(key []byte, text string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	msg := Pad([]byte(text))
	// Safe addition using math/big
	totalSize := new(big.Int).SetInt64(int64(aes.BlockSize))
	totalSize.Add(totalSize, new(big.Int).SetInt64(int64(len(msg))))

	// Check for potential overflow or wraparound
	if totalSize.Sign() <= 0 || totalSize.BitLen() > 63 {
		return nil, fmt.Errorf("size calculation overflow or wraparound")
	}

	ciphertext := make([]byte, totalSize.Int64())
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(msg))
	finalMsg := removeBase64Padding(base64.URLEncoding.EncodeToString(ciphertext))
	return []byte(finalMsg), nil
}

func decrypt(key []byte, text string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	decodedMsg, err := base64.URLEncoding.DecodeString(addBase64Padding(text))
	if err != nil {
		return nil, err
	}

	if len(decodedMsg) < aes.BlockSize || (len(decodedMsg)%aes.BlockSize) != 0 {
		return nil, errors.New("blocksize must be multipe of decoded message length")
	}

	iv := decodedMsg[:aes.BlockSize]
	msg := decodedMsg[aes.BlockSize:]

	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(msg, msg)

	unpadMsg, err := Unpad(msg)
	if err != nil {
		return nil, err
	}

	return (unpadMsg), nil
}

func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	return encrypt(key, string(plaintext))
}

func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	return decrypt(key, string(ciphertext))
}

func encryptGCM(plaintext, key []byte, storeName string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, gcmAdditionalData(storeName))
	combined := append(nonce, sealed...)
	return []byte(gcmPayloadPrefix + base64.RawURLEncoding.EncodeToString(combined)), nil
}

func decryptGCM(encoded string, key []byte, storeName string) ([]byte, error) {
	if !strings.HasPrefix(encoded, gcmPayloadPrefix) {
		return nil, errors.New("authenticated ciphertext is missing its format marker")
	}
	encoded = strings.TrimPrefix(encoded, gcmPayloadPrefix)
	combined, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode authenticated ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(combined) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("authenticated ciphertext is too short")
	}
	nonce := combined[:gcm.NonceSize()]
	ciphertext := combined[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, gcmAdditionalData(storeName))
	if err != nil {
		return nil, errors.New("authenticated ciphertext verification failed")
	}
	return plaintext, nil
}
