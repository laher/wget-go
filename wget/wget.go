package wget

import (
	"errors"
	"fmt"
	"github.com/laher/uggo"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//TODO
// ftp (3rd party supp?)
// clobber behaviour
// limit-rate
// timeouts - connect,read,dns?
// wait,waitretry
// proxies/?
// quota/?
// user/password/ask-password
// certificates/no-check-certificate ...
// exit statuses
// recursive downloads
// timestamping
// wgetrc
type WgetOptions struct {
	IsContinue     bool
	OutputFilename string
	Timeout        int //TODO
	Retries        int //TODO
	IsVerbose      bool //todo
	DefaultPage    string
	UserAgent      string //todo
	ProxyUser	string //todo
	ProxyPassword	string //todo
	Referer		string //todo
	SaveHeaders	bool //todo
	PostData	string //todo
	HttpUser	string //todo
	HttpPassword    string //todo
	IsNoCheckCertificate bool
	SecureProtocol string
}

const (
	VERSION              = "0.1.3"
	FILEMODE os.FileMode = 0660
)

func Wget(call []string) error {

	options := WgetOptions{}
	flagSet := uggo.NewFlagSetDefault("wget", "[options] URL", VERSION)
	flagSet.AliasedBoolVar(&options.IsContinue, []string{"c", "continue"}, false, "continue")
	flagSet.AliasedStringVar(&options.OutputFilename, []string{"O","output-document"}, "", "specify filename")
	flagSet.StringVar(&options.DefaultPage, "default-page", "index.html", "default page name")
	flagSet.BoolVar(&options.IsNoCheckCertificate, "no-check-certificate", false, "skip certificate checks")
	//flagSet.BoolVar(&options.IsVerbose, "v", false, "verbose")
	//some options are implemented in go-1.2+ only
	extraOptions(flagSet, options)
	err := flagSet.Parse(call[1:])
	if err != nil {
		flagSet.Usage()
		fmt.Fprintf(os.Stderr, "Flag error:  %v\n\n", err.Error())
		return err
	}
	if flagSet.ProcessHelpOrVersion() {
		return nil
	}
	args := flagSet.Args()
	if len(args) < 1 {
		flagSet.Usage()
		return errors.New("Not enough args")
	}
	if len(args) > 0 {
		links := args
		return wget(links, options)
	} else {
		if uggo.IsPipingStdin() {
			//check STDIN
			return wget([]string{}, options)
		} else {
			//NOT piping.
			flagSet.Usage()
			return errors.New("Not enough args")
		}
	}
}

func wget(links []string, options WgetOptions) error {
	for _, link := range links {
		err := wgetOne(link, options)
		if err != nil {
			return err
		}
	}
	return nil
}

func tidyFilename(filename, defaultFilename string) string {
	//invalid filenames ...
	if filename == "" || filename == "/" || filename == "\\" || filename == "." {
		filename = defaultFilename
		//filename = "index"
	}
	return filename
}

func wgetOne(link string, options WgetOptions) error {
	if !strings.Contains(link, ":") {
		link = "http://" + link
	}
	startTime := time.Now()
	request, err := http.NewRequest("GET", link, nil)
	//resp, err := http.Get(link)
	if err != nil {
		return err
	}

	filename := ""
	if options.OutputFilename != "" {
		filename = options.OutputFilename
	}

	tr, err := getHttpTransport(options)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: tr}

	//continue from where we left off ...
	if options.IsContinue {
		//not specified
		if filename == "" {
			filename = filepath.Base(request.URL.Path)
			filename = tidyFilename(filename, options.DefaultPage)
		
		}
		if !strings.Contains(filename, ".") {
			filename = filename + ".html"
		}
		fi, err := os.Stat(filename)
		if err != nil {
			return err
		}
		from := fi.Size()
		rangeHeader := fmt.Sprintf("bytes=%d-", from)
		/*
			NOTE: this could be added as an option later. ...
			//not needed
			headRequest, err := http.NewRequest("HEAD", link, nil)
			if err != nil {
				return err
			}
			headResp, err := client.Do(headRequest)
			if err != nil {
				return err
			}

			cl := headResp.Header.Get("Content-Length")
			if cl != "" {
				rangeHeader = fmt.Sprintf("bytes=%d-%s", from, cl)
				if options.IsVerbose {
					fmt.Printf("Adding range header: %s\n", rangeHeader)
				}
			} else {
				fmt.Println("Could not find file length using HEAD request")
			}
		*/
		request.Header.Add("Range", rangeHeader)
	}
	if options.IsVerbose {
		for headerName, headerValue := range request.Header {
			fmt.Printf("Request header %s: %s\n", headerName, headerValue)
		}
	}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Printf("Http response status: %s\n", resp.Status)
	if options.IsVerbose {
		for headerName, headerValue := range resp.Header {
			fmt.Printf("Response header %s: %s\n", headerName, headerValue)
		}
	}
	lenS := resp.Header.Get("Content-Length")
	length := int64(-1)
	if lenS != "" {
		length, err = strconv.ParseInt(lenS, 10, 32)
		if err != nil {
			return err
		}
	}

	typ := resp.Header.Get("Content-Type")
	fmt.Printf("Content-Length: %v Content-Type: %s\n", lenS, typ)

	if filename == "" {
		filename, err = getFilename(request, resp, options)
		if err != nil {
			return err
		}
	}

	contentRange := resp.Header.Get("Content-Range")
	rangeEffective := false
	if contentRange != "" {
		//TODO parse it?
		rangeEffective = true
	} else if options.IsContinue {
		fmt.Printf("Range request did not produce a Content-Range response\n")
	}
	fmt.Printf("Saving to: '%v'\n\n", filename)
	var openFlags int
	if options.IsContinue && rangeEffective {
		openFlags = os.O_WRONLY | os.O_APPEND
	} else {
		openFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	out, err := os.OpenFile(filename, openFlags, FILEMODE)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 4068)
	tot := int64(0)
	i := 0

	for {
		// read a chunk
		n, err := resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		tot += int64(n)

		// write a chunk
		if _, err := out.Write(buf[:n]); err != nil {
			return err
		}
		i += 1
		if length > -1 {
			if length < 1 {
				fmt.Printf("\r     [ <=>                                  ] %d\t-.--KB/s eta ?s             ", tot)
			} else {
				//show percentage
				perc := (100 * tot) / length
				prog := progress(perc)
				nowTime := time.Now()
				totTime := nowTime.Sub(startTime)
				spd := float64(tot/1000) / totTime.Seconds()
				remKb := float64(length-tot) / float64(1000)
				eta := remKb / spd
				fmt.Printf("\r%3d%% [%s] %d\t%0.2fKB/s eta %0.1fs             ", perc, prog, tot, spd, eta)
			}
		} else {
			//show dots
			if math.Mod(float64(i), 20) == 0 {
				fmt.Print(".")
			}
		}
	}
	
	nowTime := time.Now()
	totTime := nowTime.Sub(startTime)
	spd := float64(tot/1000) / totTime.Seconds()
	if length < 1 {
		fmt.Printf("\r     [ <=>                                  ] %d\t-.--KB/s in %0.1fs             ", tot, totTime.Seconds())
		fmt.Printf("\n (%0.2fKB/s) - '%v' saved [%v]\n", spd, filename, tot)
	} else {
		perc := (100 * tot) / length
		prog := progress(perc)
		fmt.Printf("\r%3d%% [%s] %d\t%0.2fKB/s in %0.1fs             ", perc, prog, tot, spd, totTime.Seconds())
		fmt.Printf("\n '%v' saved [%v/%v]\n", filename, tot, length)
	}
	if err != nil {
		return err
	}
	err = out.Close()
	return err
}

func progress(perc int64) string {
	equalses := perc * 38 / 100
	if equalses < 0 {
		equalses = 0
	}
	spaces := 38 - equalses
	if spaces < 0 {
		spaces = 0
	}
	prog := strings.Repeat("=", int(equalses)) + ">" + strings.Repeat(" ", int(spaces))
	return prog
}

func getFilename(request *http.Request, resp *http.Response, options WgetOptions) (string, error) {
	filename := filepath.Base(request.URL.Path)

	if !strings.Contains(filename, ".") {
		//original link didnt represent the file type. Try using the response url (after redirects)
		filename = filepath.Base(resp.Request.URL.Path)
	}
	filename = tidyFilename(filename, options.DefaultPage)

	if !strings.Contains(filename, ".") {
		ct := resp.Header.Get("Content-Type")
		//println(ct)
		ext := "htm"
		mediatype, _, err := mime.ParseMediaType(ct)
		if err != nil {
			fmt.Printf("mime error: %v\n", err)
		} else {
			fmt.Printf("mime type: %v (from Content-Type %v)\n", mediatype, ct)
			slash := strings.Index(mediatype, "/")
			if slash != -1 {
				_, sub := mediatype[:slash], mediatype[slash+1:]
				if sub != "" {
					ext = sub
				}
			}
		}
		filename = filename + "." + ext
	}
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return filename, nil
		} else {
			return "", err
		}
	} else {
		num := 1
		//just stop after 100
		for num < 100 {
			filenameNew := filename + "." + strconv.Itoa(num)
			_, err := os.Stat(filenameNew)
			if err != nil {
				if os.IsNotExist(err) {
					return filenameNew, nil
				} else {
					return "", err
				}
			}
			num += 1
		}
		return filename, errors.New("Stopping after trying 100 filename variants")
	}
}
