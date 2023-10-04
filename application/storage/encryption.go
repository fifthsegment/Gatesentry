package gatesentry2storage;

// import (
// 	"crypto/aes"
// 	"crypto/cipher"
// 	// "encoding/hex"
// 	"crypto/rand"
//     // "encoding/base64"
// 	"io"
// 	"fmt"
// 	// "errors"
// )

import (
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    // "fmt"
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
    ciphertext := make([]byte, aes.BlockSize+len(msg))
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

func Encrypt(plaintext []byte, key []byte) ([]byte, error){
	return encrypt(key, string(plaintext) );
	// // return encrypt( string(plaintext), string(key) );
	// // The key argument should be the AES key, either 16 or 32 bytes
	// // to select AES-128 or AES-256.
	// // key := []byte("AES256Key-32Characters1234567890")
	// // plaintext := []byte("exampleplaintext")

	// block, err := aes.NewCipher(key)
	// if err != nil {
	// 	return nil, err;
	// }

	// // Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	// nonce := []byte("ABCDEFGHIJKL");
	// _=nonce;
	// // nonce := make([]byte, 12)
	// // if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
	// // 	panic(err.Error())
	// // }

	// aesgcm, err := cipher.NewGCM(block)
	// if err != nil {
	// 	return nil, err;
	// }

	// ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	// fmt.Println("Encrypted text = "+ string( ciphertext ) );
	// // fmt.Printf("%x\n", ciphertext)
	// return ciphertext,nil;
}

func Decrypt(ciphertext []byte, key []byte) ([]byte, error ){
	return decrypt(key, string(ciphertext) );
	// // return decrypt( string(ciphertext), string(key) );
	// // The key argument should be the AES key, either 16 or 32 bytes
	// // to select AES-128 or AES-256.
	// // key := []byte("AES256Key-32Characters1234567890")
	// // ciphertext, _ := hex.DecodeString("f90fbef747e7212ad7410d0eee2d965de7e890471695cddd2a5bc0ef5da1d04ad8147b62141ad6e4914aee8c512f64fba9037603d41de0d50b718bd665f019cdcd")

	// nonce := []byte("ABCDEFGHIJKL");
	// _=nonce;

	// block, err := aes.NewCipher(key)
	// if err != nil {

	// 	fmt.Println("key error ")
	// 	return nil, err;
	// }

	// aesgcm, err := cipher.NewGCM(block)

	// if err != nil {
	// 	fmt.Println("block error ")
	// 	return nil, err;
	// }

	// plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	// if err != nil {
	// 	fmt.Println("last stage error " + string(plaintext) )
	// 	return nil, err;
	// }

	// // fmt.Printf("%s\n", string(plaintext))

	// return plaintext,nil;
}