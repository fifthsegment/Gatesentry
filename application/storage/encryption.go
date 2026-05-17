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
	unpadding := int(src[length-1])

	if unpadding > length {
		return nil, errors.New("unpad error. This could happen when incorrect encryption key is used")
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

	if (len(decodedMsg) % aes.BlockSize) != 0 {
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
