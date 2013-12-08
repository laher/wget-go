// +build !go1.2

package wget

import(
	"crypto/tls"
	"github.com/laher/uggo"
	"net/http"
)

func extraOptions(flagSet uggo.FlagSetWithAliases, options WgetOptions) {
}

func getHttpTransport(options WgetOptions) (*http.Transport, error) {
	tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: options.IsNoCheckCertificate}}
	return tr, nil

}
