package headers

import (
	"fmt"
	"strings"

	"github.com/mrHChen/goutils/stream/base"
	"github.com/mrHChen/goutils/stream/utils"
)

// AuthMethod 认证方式
type AuthMethod int

const (
	// AuthBasic 基本认证方法
	AuthBasic AuthMethod = iota
	// AuthDigest 摘要认证
	AuthDigest
)

// Read 解码Authenticate或WWW-Authenticate报头
func (h *Authenticate) Read(v base.HeaderValue) error {
	if len(v) == 0 {
		return fmt.Errorf("value not provided")
	}

	if len(v) > 1 {
		return fmt.Errorf("value provided multiple times (%v)", v)
	}

	v0 := v[0]

	i := strings.IndexByte(v0, ' ')
	if i < 0 {
		return fmt.Errorf("unable to split between method and keys (%v)", v0)
	}
	method, v0 := v0[:i], v0[i+1:]

	switch method {
	case "Basic":
		h.Method = AuthBasic

	case "Digest":
		h.Method = AuthDigest

	default:
		return fmt.Errorf("invalid method (%s)", method)
	}

	kvs, err := utils.KeyValParse(v0, ',')
	if err != nil {
		return err
	}

	for k, rv := range kvs {
		v := rv

		switch k {
		case "username":
			h.Username = &v

		case "realm":
			h.Realm = &v

		case "nonce":
			h.Nonce = &v

		case "uri":
			h.URI = &v

		case "response":
			h.Response = &v

		case "opaque":
			h.Opaque = &v

		case "stale":
			h.Stale = &v

		case "algorithm":
			h.Algorithm = &v
		}
	}

	return nil
}

// Authenticate 报文内容
type Authenticate struct {
	// 认证方式
	Method AuthMethod

	// username
	Username *string

	// realm
	Realm *string

	// (optional) nonce
	Nonce *string

	// (optional) uri
	URI *string

	// (optional) response
	Response *string

	// (optional) opaque
	Opaque *string

	// (optional) stale
	Stale *string

	// (optional) algorithm
	Algorithm *string
}

// Write 对Authenticate或WWW-Authenticate报头进行编码
func (h Authenticate) Write() base.HeaderValue {
	ret := ""

	switch h.Method {
	case AuthBasic:
		ret += "Basic"

	case AuthDigest:
		ret += "Digest"
	}

	ret += " "

	var rets []string

	if h.Username != nil {
		rets = append(rets, "username=\""+*h.Username+"\"")
	}

	if h.Realm != nil {
		rets = append(rets, "realm=\""+*h.Realm+"\"")
	}

	if h.Nonce != nil {
		rets = append(rets, "nonce=\""+*h.Nonce+"\"")
	}

	if h.URI != nil {
		rets = append(rets, "uri=\""+*h.URI+"\"")
	}

	if h.Response != nil {
		rets = append(rets, "response=\""+*h.Response+"\"")
	}

	if h.Opaque != nil {
		rets = append(rets, "opaque=\""+*h.Opaque+"\"")
	}

	if h.Stale != nil {
		rets = append(rets, "stale=\""+*h.Stale+"\"")
	}

	if h.Algorithm != nil {
		rets = append(rets, "algorithm=\""+*h.Algorithm+"\"")
	}

	ret += strings.Join(rets, ", ")

	return base.HeaderValue{ret}
}
