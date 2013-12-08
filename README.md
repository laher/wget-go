wget.go
=======

wget, partially re-implemented in go.

### Implemented so far

 * Standard http & https requests
 * Progress bar and speed reporting. tested on Linux & Windows
 * --output-document
 * --continue
 * SSL: --no-check-certificate & --secure-protocol
 * --default-page=index.html

### To do
 
 * --http-user , --http-password --auth-no-challenge
 * --proxy-user, --proxy-password
 * --header, --save-headers, --referer, --user-agent, --post-data, --content-disposition
 * --certificate,--ca-certificate,...

### Not planned
 
 * ftp protocol
 * --recursive (for archiving websites) and --warc- options

### Planned non-standard features
 * parallelize multiple requests
 * Hopefully: parallelize a large request using 'Range' header, much like --continue
 * 'known hosts' support for SSL
