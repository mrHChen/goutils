package base

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"

	"goutils/stream/utils"
)

// StatusCode is the status code of a RTSP response.
type StatusCode int

// standard status codes
const (
	StatusContinue                           StatusCode = 100
	StatusOK                                 StatusCode = 200
	StatusMovedPermanently                   StatusCode = 301
	StatusFound                              StatusCode = 302
	StatusSeeOther                           StatusCode = 303
	StatusNotModified                        StatusCode = 304
	StatusUseProxy                           StatusCode = 305
	StatusBadRequest                         StatusCode = 400
	StatusUnauthorized                       StatusCode = 401
	StatusPaymentRequired                    StatusCode = 402
	StatusForbidden                          StatusCode = 403
	StatusNotFound                           StatusCode = 404
	StatusMethodNotAllowed                   StatusCode = 405
	StatusNotAcceptable                      StatusCode = 406
	StatusProxyAuthRequired                  StatusCode = 407
	StatusRequestTimeout                     StatusCode = 408
	StatusGone                               StatusCode = 410
	StatusPreconditionFailed                 StatusCode = 412
	StatusRequestEntityTooLarge              StatusCode = 413
	StatusRequestURITooLong                  StatusCode = 414
	StatusUnsupportedMediaType               StatusCode = 415
	StatusParameterNotUnderstood             StatusCode = 451
	StatusNotEnoughBandwidth                 StatusCode = 453
	StatusSessionNotFound                    StatusCode = 454
	StatusMethodNotValidInThisState          StatusCode = 455
	StatusHeaderFieldNotValidForResource     StatusCode = 456
	StatusInvalidRange                       StatusCode = 457
	StatusParameterIsReadOnly                StatusCode = 458
	StatusAggregateOperationNotAllowed       StatusCode = 459
	StatusOnlyAggregateOperationAllowed      StatusCode = 460
	StatusUnsupportedTransport               StatusCode = 461
	StatusDestinationUnreachable             StatusCode = 462
	StatusDestinationProhibited              StatusCode = 463
	StatusDataTransportNotReadyYet           StatusCode = 464
	StatusNotificationReasonUnknown          StatusCode = 465
	StatusKeyManagementError                 StatusCode = 466
	StatusConnectionAuthorizationRequired    StatusCode = 470
	StatusConnectionCredentialsNotAccepted   StatusCode = 471
	StatusFailureToEstablishSecureConnection StatusCode = 472
	StatusInternalServerError                StatusCode = 500
	StatusNotImplemented                     StatusCode = 501
	StatusBadGateway                         StatusCode = 502
	StatusServiceUnavailable                 StatusCode = 503
	StatusGatewayTimeout                     StatusCode = 504
	StatusRTSPVersionNotSupported            StatusCode = 505
	StatusOptionNotSupported                 StatusCode = 551
	StatusProxyUnavailable                   StatusCode = 553
)

// StatusMessages contains the status messages associated with each status code.
var StatusMessages = statusMessages

var statusMessages = map[StatusCode]string{
	StatusContinue: "Continue",

	StatusOK: "OK",

	StatusMovedPermanently: "Moved Permanently",
	StatusFound:            "Found",
	StatusSeeOther:         "See Other",
	StatusNotModified:      "Not Modified",
	StatusUseProxy:         "Use Proxy",

	StatusBadRequest:                         "Bad Request",
	StatusUnauthorized:                       "Unauthorized",
	StatusPaymentRequired:                    "Payment Required",
	StatusForbidden:                          "Forbidden",
	StatusNotFound:                           "Not Found",
	StatusMethodNotAllowed:                   "Method Not Allowed",
	StatusNotAcceptable:                      "Not Acceptable",
	StatusProxyAuthRequired:                  "Proxy Auth Required",
	StatusRequestTimeout:                     "Request Timeout",
	StatusGone:                               "Gone",
	StatusPreconditionFailed:                 "Precondition Failed",
	StatusRequestEntityTooLarge:              "Request Entity Too Large",
	StatusRequestURITooLong:                  "Request URI Too Long",
	StatusUnsupportedMediaType:               "Unsupported Media Type",
	StatusParameterNotUnderstood:             "Parameter Not Understood",
	StatusNotEnoughBandwidth:                 "Not Enough Bandwidth",
	StatusSessionNotFound:                    "Session Not Found",
	StatusMethodNotValidInThisState:          "Method Not Valid In This State",
	StatusHeaderFieldNotValidForResource:     "Header Field Not Valid for Resource",
	StatusInvalidRange:                       "Invalid Range",
	StatusParameterIsReadOnly:                "Parameter Is Read-Only",
	StatusAggregateOperationNotAllowed:       "Aggregate Operation Not Allowed",
	StatusOnlyAggregateOperationAllowed:      "Only Aggregate Operation Allowed",
	StatusUnsupportedTransport:               "Unsupported Transport",
	StatusDestinationUnreachable:             "Destination Unreachable",
	StatusDestinationProhibited:              "Destination Prohibited",
	StatusDataTransportNotReadyYet:           "Data Transport Not Ready Yet",
	StatusNotificationReasonUnknown:          "Notification Reason Unknown",
	StatusKeyManagementError:                 "Key Management Error",
	StatusConnectionAuthorizationRequired:    "Connection Authorization Required",
	StatusConnectionCredentialsNotAccepted:   "Connection Credentials Not Accepted",
	StatusFailureToEstablishSecureConnection: "Failure to Establish Secure Connection",

	StatusInternalServerError:     "Internal Server Error",
	StatusNotImplemented:          "Not Implemented",
	StatusBadGateway:              "Bad Gateway",
	StatusServiceUnavailable:      "Service Unavailable",
	StatusGatewayTimeout:          "Gateway Timeout",
	StatusRTSPVersionNotSupported: "RTSP Version Not Supported",
	StatusOptionNotSupported:      "Option Not Supported",
	StatusProxyUnavailable:        "Proxy Unavailable",
}

// Response RTSP响应
type Response struct {
	// numeric status code
	StatusCode StatusCode

	// status message
	StatusMessage string

	// map of header values
	Header Header

	// optional body
	Body []byte
}

// Read 读取响应
func (r Response) Read(br *bufio.Reader) (*Response, error) {
	b, err := utils.ReadBytesLimited(br, ' ', 255)
	if err != nil {
		return nil, err
	}

	proto := string(b[:len(b)-1])

	if proto != RtspProtocol10 {
		return nil, fmt.Errorf("expected '%s', got '%s'", RtspProtocol10, proto)
	}

	b, err = utils.ReadBytesLimited(br, ' ', 4)
	if err != nil {
		return nil, err
	}
	statusCodeStr := string(b[:len(b)-1])

	statusCode64, err := strconv.ParseInt(statusCodeStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("unable to parse status code")
	}
	r.StatusCode = StatusCode(statusCode64)

	b, err = utils.ReadBytesLimited(br, '\r', 255)
	if err != nil {
		return nil, err
	}
	r.StatusMessage = string(b[:len(b)-1])

	if len(r.StatusMessage) == 0 {
		return nil, fmt.Errorf("empty status message")
	}

	err = utils.ReadByteEqual(br, '\n')
	if err != nil {
		return nil, err
	}

	err = r.Header.read(br)
	if err != nil {
		return nil, err
	}

	err = (*body)(&r.Body).read(r.Header, br)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Write writes a Response.
func (r Response) Write(bw *bufio.Writer) error {
	if r.StatusMessage == "" {
		if status, ok := statusMessages[r.StatusCode]; ok {
			r.StatusMessage = status
		}
	}

	_, err := bw.Write([]byte(RtspProtocol10 + " " + strconv.FormatInt(int64(r.StatusCode), 10) + " " + r.StatusMessage + "\r\n"))
	if err != nil {
		return err
	}

	if len(r.Body) != 0 {
		r.Header["Content-Length"] = HeaderValue{strconv.FormatInt(int64(len(r.Body)), 10)}
	}

	err = r.Header.write(bw)
	if err != nil {
		return err
	}

	err = body(r.Body).write(bw)
	if err != nil {
		return err
	}

	return bw.Flush()
}

// String implements fmt.Stringer.
func (r Response) String() string {
	buf := bytes.NewBuffer(nil)
	r.Write(bufio.NewWriter(buf))
	return buf.String()
}
