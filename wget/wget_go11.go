// +build !go1.2

package wget

import(
	"crypto/tls"
	"github.com/laher/uggo"
	"net/http"
)

func extraOptions(flagSet uggo.FlagSetWithAliases, options *Wgetter) {
}

func getHttpTransport(options *Wgetter) (*http.Transport, error) {
	tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: options.IsNoCheckCertificate}}
	return tr, nil

}
