package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucas-clemente/quic-go/http3"
	"github.com/lvht/ssltun"
	"golang.org/x/crypto/acme/autocert"
)

var name, key, root string
var h2 bool

func init() {
	flag.StringVar(&name, "name", "", "server domain name")
	flag.StringVar(&key, "key", "", "server auth key")
	flag.StringVar(&root, "root", "", "static server root")
	flag.BoolVar(&h2, "h2", false, "enable http/2 protocol")
}

func main() {
	flag.Parse()
	if name == "" || key == "" {
		flag.Usage()
		return
	}

	names := strings.Split(name, ",")

	dir := os.Getenv("HOME") + "/.autocert"
	acm := autocert.Manager{
		Cache:      autocert.DirCache(dir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(names...),
	}
	tlsCfg := acm.TLSConfig()

	if !h2 {
		tlsCfg.NextProtos = []string{"http/1.1", "acme-tls/1"}
	}

	ln, err := tls.Listen("tcp", ":443", tlsCfg)
	if err != nil {
		log.Fatal(err)
	}

	proxy := &ssltun.Proxy{DomainNames: names}
	proxy.Auth = func(u, p string) bool { return u == key }
	if root != "" {
		proxy.FileHandlers = make(map[string]http.Handler, len(names))
		for _, name := range names {
			path := filepath.Join(root, name)
			proxy.FileHandlers[name] = http.FileServer(http.Dir(path))
		}
	}

	go func() {
		tlsCfg := acm.TLSConfig()
		tlsCfg.NextProtos = []string{"h3", "h3-29", "h3-32", "h3-34"}

		ln, err := net.ListenPacket("udp", ":4430")
		if err != nil {
			log.Fatal(err)
		}

		f := func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("hello h3 and svcb"))
		}

		h := http.HandlerFunc(f)

		h3 := http3.Server{Server: &http.Server{Handler: h}}
		h3.TLSConfig = tlsCfg
		h3.Serve(ln)
	}()

	if err = http.Serve(ln, proxy); err != nil {
		log.Fatal(err)
	}
}
