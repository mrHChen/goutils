package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func Md5Hex(in string) string {
	h := md5.New()
	h.Write([]byte(in))
	return hex.EncodeToString(h.Sum(nil))
}

func Sha256Base64(in string) string {
	h := sha256.New()
	h.Write([]byte(in))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
