package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"

	"kkfileview/internal/config"
)

// AESDecrypt 对应 Java AESUtil.AesDecrypt：AES/ECB/PKCS5，输入为 Base64 串
func AESDecrypt(encrypted string) (string, error) {
	cfg := configAesKey()
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encrypted, " ", "+"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(cfg))
	if err != nil {
		return "", err
	}
	if len(raw) < aes.BlockSize || len(raw)%aes.BlockSize != 0 {
		return "", errInvalidAes
	}
	decrypted := make([]byte, len(raw))
	mode := newECBDecrypter(block)
	mode.CryptBlocks(decrypted, raw)
	// 去 PKCS5 填充
	pad := int(decrypted[len(decrypted)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(decrypted) {
		return "", errInvalidAes
	}
	return string(decrypted[:len(decrypted)-pad]), nil
}

// AESEncrypt 对应 Java AESUtil.AesEncrypt
func AESEncrypt(plaintext string) (string, error) {
	cfg := configAesKey()
	block, err := aes.NewCipher([]byte(cfg))
	if err != nil {
		return "", err
	}
	src := pkcs5Padding([]byte(plaintext), block.BlockSize())
	encrypted := make([]byte, len(src))
	newECBEncrypter(block).CryptBlocks(encrypted, src)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

type errInvalidAesT struct{}

func (errInvalidAesT) Error() string { return "AES解密异常" }

var errInvalidAes = errInvalidAesT{}

func pkcs5Padding(src []byte, blockSize int) []byte {
	pad := blockSize - len(src)%blockSize
	for i := 0; i < pad; i++ {
		src = append(src, byte(pad))
	}
	return src
}

func configAesKey() string {
	k := config.Get().AesKey
	if len(k) > 16 {
		k = k[:16]
	}
	for len(k) < 16 {
		k += "0"
	}
	return k
}

// ECB 模式实现（Go 标准库不含 ECB）

type ecbDecrypter struct{ b cipher.Block }
type ecbEncrypter struct{ b cipher.Block }

func newECBDecrypter(b cipher.Block) ecbDecrypter { return ecbDecrypter{b: b} }
func newECBEncrypter(b cipher.Block) ecbEncrypter { return ecbEncrypter{b: b} }

func (e ecbDecrypter) CryptBlocks(dst, src []byte) {
	for i := 0; i < len(src); i += e.b.BlockSize() {
		e.b.Decrypt(dst[i:i+e.b.BlockSize()], src[i:i+e.b.BlockSize()])
	}
}

func (e ecbEncrypter) CryptBlocks(dst, src []byte) {
	for i := 0; i < len(src); i += e.b.BlockSize() {
		e.b.Encrypt(dst[i:i+e.b.BlockSize()], src[i:i+e.b.BlockSize()])
	}
}
