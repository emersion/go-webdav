package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jonyTF/go-webdav"
)

func main() {
	var addr string
	flag.StringVar(&addr, "addr", ":8080", "listening address")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [options...] [directory]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	path := flag.Arg(0)
	if path == "" {
		path = "."
	}

	handler := webdav.Handler{
		FileSystem: webdav.LocalFileSystem(path),
	}
	log.Printf("WebDAV server listening on %v", addr)
	log.Fatal(http.ListenAndServe(addr, &handler))
}
