package headers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"goutils/stream/base"
	"goutils/stream/utils"
)

// Sender allows to generate credentials for a Validator.
type Sender struct {
	user   string
	pass   string
	method AuthMethod
	auth   Authenticate
}

// NewSender allocates a Sender with the WWW-Authenticate header provided by
// a Validator and a set of credentials.
func NewSender(v base.HeaderValue, user string, pass string) (*Sender, error) {
	// prefer digest
	if v0 := func() string {
		for _, vi := range v {
			if strings.HasPrefix(vi, "Digest") {
				return vi
			}
		}
		return ""
	}(); v0 != "" {
		var auth Authenticate
		err := auth.Read(base.HeaderValue{v0})
		if err != nil {
			return nil, err
		}

		if auth.Realm == nil {
			return nil, fmt.Errorf("realm is missing")
		}

		if auth.Nonce == nil {
			return nil, fmt.Errorf("nonce is missing")
		}

		return &Sender{
			user:   user,
			pass:   pass,
			method: AuthDigest,
			auth:   auth,
		}, nil
	}

	if v0 := func() string {
		for _, vi := range v {
			if strings.HasPrefix(vi, "Basic") {
				return vi
			}
		}
		return ""
	}(); v0 != "" {
		var auth Authenticate
		err := auth.Read(base.HeaderValue{v0})
		if err != nil {
			return nil, err
		}

		if auth.Realm == nil {
			return nil, fmt.Errorf("realm is missing")
		}

		return &Sender{
			user:   user,
			pass:   pass,
			method: AuthBasic,
			auth:   auth,
		}, nil
	}

	return nil, fmt.Errorf("no authentication methods available")
}

// AddAuthorization adds the Authorization header to a Request.
func (se *Sender) AddAuthorization(req *base.Request) {
	urStr := req.URL.String()

	h := Authorization{
		Method: se.method,
	}

	switch se.method {
	case AuthBasic:
		h.BasicUser = se.user
		h.BasicPass = se.pass

	default: // headers.AuthDigest
		response := utils.Md5Hex(
			utils.Md5Hex(se.user+":"+*se.auth.Realm+":"+se.pass) +
				":" +
				*se.auth.Nonce +
				":" +
				utils.Md5Hex(string(req.Method)+":"+urStr))

		h.DigestValues = Authenticate{
			Method:   AuthDigest,
			Username: &se.user,
			Realm:    se.auth.Realm,
			Nonce:    se.auth.Nonce,
			URI:      &urStr,
			Response: &response,
		}
	}

	if req.Header == nil {
		req.Header = make(base.Header)
	}

	req.Header["Authorization"] = h.Write()
}

func (se *Sender) CheckAuth(url *url.URL) error {

	urStr := url.String()

	response := utils.Md5Hex(utils.Md5Hex(se.user+":"+*se.auth.Realm+":"+se.pass) +
		":" +
		*se.auth.Nonce +
		":" +
		utils.Md5Hex(string(se.method)+
			":"+
			urStr))

	if response != *se.auth.Response {
		return errors.New(" response not equal ")
	}
	return nil
}
