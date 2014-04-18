// +build go1.2

package wget

import(
	"crypto/tls"
	"errors"
	"github.com/laher/uggo"
	"net/http"
)

func extraOptions(flagSet uggo.FlagSetWithAliases, options *Wgetter) {
	flagSet.StringVar(&options.SecureProtocol, "secure-protocol", "auto", "secure protocol to be used (auto/SSLv2/SSLv3/TLSv1)")
}

func getHttpTransport(options *Wgetter) (*http.Transport, error) {
	minSecureProtocol := uint16(0)
	maxSecureProtocol := uint16(0)
	switch options.SecureProtocol {
	case "auto":
		maxSecureProtocol = minSecureProtocol
	case "SSLv3":
		minSecureProtocol = tls.VersionSSL30
		maxSecureProtocol = minSecureProtocol
	case "TLSv1":
		minSecureProtocol = tls.VersionTLS10
	case "":
		//OK
	default:
		return nil, errors.New("unrecognised secure protocol '"+ options.SecureProtocol+"'")
	}
	tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: options.IsNoCheckCertificate, MinVersion: minSecureProtocol, MaxVersion: maxSecureProtocol}}
	return tr, nil

}
