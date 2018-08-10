// Wgetter downloads and saves/pipes HTTP requests
package wget

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/laher/uggo"
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
type Wgetter struct {
	IsContinue bool
	// should be set explicitly to false when running from CLI. uggo will detect as best as possible
	AlwaysPipeStdin      bool
	OutputFilename       string
	Timeout              int  //TODO
	Retries              int  //TODO
	IsVerbose            bool //todo
	DefaultPage          string
	UserAgent            string //todo
	ProxyUser            string //todo
	ProxyPassword        string //todo
	Referer              string //todo
	SaveHeaders          bool   //todo
	PostData             string //todo
	HttpUser             string //todo
	HttpPassword         string //todo
	IsNoCheckCertificate bool
	SecureProtocol       string

	links []string
}

const (
	VERSION              = "0.5.0"
	FILEMODE os.FileMode = 0660
)

//Factory for wgetter which outputs to Stdout
func WgetToOut(urls ...string) *Wgetter {
	wgetter := Wget(urls...)
	wgetter.OutputFilename = "-"
	return wgetter
}

// Factory for wgetter
func Wget(urls ...string) *Wgetter {
	wgetter := new(Wgetter)
	wgetter.links = urls
	if len(urls) == 0 {
		wgetter.AlwaysPipeStdin = true
	}
	return wgetter
}

// CLI invocation for wgetter
func WgetCli(call []string) (error, int) {
	inPipe := os.Stdin
	outPipe := os.Stdout
	errPipe := os.Stderr
	wgetter := new(Wgetter)
	wgetter.AlwaysPipeStdin = false
	err, code := wgetter.ParseFlags(call, errPipe)
	if err != nil {
		return err, code
	}
	return wgetter.Exec(inPipe, outPipe, errPipe)
}

// Name() returns the name of the util
func (tail *Wgetter) Name() string {
	return "wget"
}

// Parse CLI flags
func (w *Wgetter) ParseFlags(call []string, errPipe io.Writer) (error, int) {

	flagSet := uggo.NewFlagSetDefault("wget", "[options] URL", VERSION)
	flagSet.SetOutput(errPipe)
	flagSet.AliasedBoolVar(&w.IsContinue, []string{"c", "continue"}, false, "continue")
	flagSet.AliasedStringVar(&w.OutputFilename, []string{"O", "output-document"}, "", "specify filename")
	flagSet.StringVar(&w.DefaultPage, "default-page", "index.html", "default page name")
	flagSet.BoolVar(&w.IsNoCheckCertificate, "no-check-certificate", false, "skip certificate checks")

	//some features are available in go-1.2+ only
	extraOptions(flagSet, w)
	err, code := flagSet.ParsePlus(call[1:])
	if err != nil {
		return err, code
	}

	//fmt.Fprintf(errPipe, "%+v\n", w)
	args := flagSet.Args()
	if len(args) < 1 {
		flagSet.Usage()
		return errors.New("Not enough args"), 1
	}
	if len(args) > 0 {
		w.links = args
		//return wget(links, w)
		return nil, 0
	} else {
		if w.AlwaysPipeStdin || uggo.IsPipingStdin() {
			//check STDIN
			//return wget([]string{}, options)
			return nil, 0
		} else {
			//NOT piping.
			flagSet.Usage()
			return errors.New("Not enough args"), 1
		}
	}
}

// Perform the wget ...
func (w *Wgetter) Exec(inPipe io.Reader, outPipe io.Writer, errPipe io.Writer) (error, int) {
	if len(w.links) > 0 {
		for _, link := range w.links {
			err := wgetOne(link, w, outPipe, errPipe)
			if err != nil {
				return err, 1
			}
		}
	} else {
		bio := bufio.NewReader(inPipe)
		hasMoreInLine := true
		var err error
		var line []byte
		for hasMoreInLine {
			line, hasMoreInLine, err = bio.ReadLine()
			if err == nil {
				//line from stdin
				err = wgetOne(strings.TrimSpace(string(line)), w, outPipe, errPipe)

				if err != nil {
					return err, 1
				}
			} else {
				//finish
				hasMoreInLine = false
			}
		}

	}
	return nil, 0
}

func tidyFilename(filename, defaultFilename string) string {
	//invalid filenames ...
	if filename == "" || filename == "/" || filename == "\\" || filename == "." {
		filename = defaultFilename
		//filename = "index"
	}
	return filename
}

func wgetOne(link string, options *Wgetter, outPipe io.Writer, errPipe io.Writer) error {
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
	//include stdout
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
		if options.OutputFilename == "-" {
			return errors.New("Continue not supported while piping")
		}
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
					fmt.Fprintf(errPipe, "Adding range header: %s\n", rangeHeader)
				}
			} else {
				fmt.Println("Could not find file length using HEAD request")
			}
		*/
		request.Header.Add("Range", rangeHeader)
	}
	if options.IsVerbose {
		for headerName, headerValue := range request.Header {
			fmt.Fprintf(errPipe, "Request header %s: %s\n", headerName, headerValue)
		}
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Fprintf(errPipe, "Http response status: %s\n", resp.Status)
	if options.IsVerbose {
		for headerName, headerValue := range resp.Header {
			fmt.Fprintf(errPipe, "Response header %s: %s\n", headerName, headerValue)
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
	fmt.Fprintf(errPipe, "Content-Length: %v Content-Type: %s\n", lenS, typ)

	if filename == "" {
		filename, err = getFilename(request, resp, options, errPipe)
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
		fmt.Fprintf(errPipe, "Range request did not produce a Content-Range response\n")
	}
	var out io.Writer
	var outFile *os.File
	if filename != "-" {
		fmt.Fprintf(errPipe, "Saving to: '%v'\n\n", filename)
		var openFlags int
		if options.IsContinue && rangeEffective {
			openFlags = os.O_WRONLY | os.O_APPEND
		} else {
			openFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		outFile, err = os.OpenFile(filename, openFlags, FILEMODE)
		if err != nil {
			return err
		}
		defer outFile.Close()
		out = outFile
	} else {
		//save to outPipe
		out = outPipe
	}
	buf := make([]byte, 4068)
	tot := int64(0)
	i := 0

	tick := make(chan struct{})
	var n, timeouts int
	for {
		go func() {
			// read a chunk
			n, err = resp.Body.Read(buf)
			tick <- struct{}{}
		}()
		select {
		case <-tick:
		case <-time.Tick(time.Second * time.Duration(options.Timeout)):
			if timeouts < options.Retries {
				c := color.New(color.FgRed)
				c.Fprintln(errPipe, "\ntime out reached, retrying...")
				continue
			}
			return errors.New("timeout")
		}
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
				fmt.Fprintf(errPipe, "\r     [ <=>                                  ] %d\t-.--KB/s eta ?s             ", tot)
			} else {
				//show percentage
				perc := (100 * tot) / length
				prog := progress(perc)
				nowTime := time.Now()
				totTime := nowTime.Sub(startTime)
				spd := float64(tot/1000) / totTime.Seconds()
				remKb := float64(length-tot) / float64(1000)
				eta := remKb / spd
				fmt.Fprintf(errPipe, "\r%3d%% [%s] %d\t%0.2fKB/s eta %0.1fs             ", perc, prog, tot, spd, eta)
			}
		} else {
			//show dots
			if math.Mod(float64(i), 20) == 0 {
				fmt.Fprint(errPipe, ".")
			}
		}
	}
	nowTime := time.Now()
	totTime := nowTime.Sub(startTime)
	spd := float64(tot/1000) / totTime.Seconds()
	if length < 1 {
		fmt.Fprintf(errPipe, "\r     [ <=>                                  ] %d\t-.--KB/s in %0.1fs             ", tot, totTime.Seconds())
		fmt.Fprintf(errPipe, "\n (%0.2fKB/s) - '%v' saved [%v]\n", spd, filename, tot)
	} else {
		perc := (100 * tot) / length
		prog := progress(perc)
		fmt.Fprintf(errPipe, "\r%3d%% [%s] %d\t%0.2fKB/s in %0.1fs             ", perc, prog, tot, spd, totTime.Seconds())
		fmt.Fprintf(errPipe, "\n '%v' saved [%v/%v]\n", filename, tot, length)
	}
	if err != nil {
		return err
	}
	if outFile != nil {
		err = outFile.Close()
	}
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

func getFilename(request *http.Request, resp *http.Response, options *Wgetter, errPipe io.Writer) (string, error) {
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
			fmt.Fprintf(errPipe, "mime error: %v\n", err)
		} else {
			fmt.Fprintf(errPipe, "mime type: %v (from Content-Type %v)\n", mediatype, ct)
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
