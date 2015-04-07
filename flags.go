package main

import (
	"flag"
	"fmt"
	"runtime"
	"strconv"
)

const (
	default_server_colon_port = ":3000"
	default_redis_colon_port  = ":6379"
)

var (
	// List of configuration filenames to check
	SERVER_CONFIGURATION_FILENAMES = []string{"/etc/algernon/server.lua"}

	// Configuration that is exposed to the server configuration script
	SERVER_DIR, SERVER_ADDR, SERVER_CERT, SERVER_KEY, SERVER_CONF_SCRIPT, SERVER_HTTP2_LOG string

	// Redis configuration
	REDIS_ADDR string
	REDIS_DB   int
)

func Usage() {
	fmt.Println("\n" + version_string + "\n\n" + description)
	// Possible arguments are also, for backward compatibility:
	// server dir, server addr, certificate file, key file, redis addr and redis db index
	// They are not mentioned here, but are possible to use, in that strict order.
	fmt.Println(`

Syntax:
  algernon [flags] [server dir] [server addr]

Possible flags:
  --version                    Show application name and version
  --dir=DIRECTORY              The server directory
  --addr=[HOST][:PORT]         Host and port the server should listen at
  --cert=FILENAME              TLS certificate, if using HTTPS
  --key=FILENAME               TLS key, if using HTTPS
  --redis=[HOST][:PORT]        Address for connecting to a remote Redis database
                               (uses port 6379 at localhost by default)
  --dbindex=INDEX              Which Redis database index to use
  --conf=FILENAME              Lua script with additional configuration
  --http2log=FILENAME          Log the (verbose) HTTP/2 log to a file
  --help                       This text
`)
}

func handleFlags() {
	flag.Usage = Usage

	// The default for running the redis server on Windows is to listen
	// to "localhost:port", but not just ":port".
	host := ""
	if runtime.GOOS == "windows" {
		host = "localhost"
	}

	// Commandline flag configuration

	flag.StringVar(&SERVER_DIR, "dir", ".", "Server directory")
	flag.StringVar(&SERVER_ADDR, "addr", host+default_server_colon_port, "Server [host][:port] (ie \":443\")")
	flag.StringVar(&SERVER_CERT, "cert", "cert.pem", "Server certificate")
	flag.StringVar(&SERVER_KEY, "key", "key.pem", "Server key")
	flag.StringVar(&REDIS_ADDR, "redis", host+default_redis_colon_port, "Redis [host][:port] (ie \":6379\")")
	flag.IntVar(&REDIS_DB, "dbindex", 0, "Redis database index")
	flag.StringVar(&SERVER_CONF_SCRIPT, "conf", "server.lua", "Server configuration")
	flag.StringVar(&SERVER_HTTP2_LOG, "http2log", "/dev/null", "HTTP/2 log")

	flag.Parse()

	// For backwards compatibility with earlier versions of algernon

	if len(flag.Args()) >= 1 {
		SERVER_DIR = flag.Args()[0]
	}
	if len(flag.Args()) >= 2 {
		SERVER_ADDR = flag.Args()[1]
	}
	if len(flag.Args()) >= 3 {
		SERVER_CERT = flag.Args()[2]
	}
	if len(flag.Args()) >= 4 {
		SERVER_KEY = flag.Args()[3]
	}
	if len(flag.Args()) >= 5 {
		REDIS_ADDR = flag.Args()[4]
	}
	if len(flag.Args()) >= 6 {
		// Convert the dbindex from string to int
		dbindex, err := strconv.Atoi(flag.Args()[5])
		if err != nil {
			REDIS_DB = dbindex
		}
	}

	// Add the SERVER_CONF_SCRIPT to the list of configuration scripts to be read and executed
	SERVER_CONFIGURATION_FILENAMES = append(SERVER_CONFIGURATION_FILENAMES, SERVER_CONF_SCRIPT)
}