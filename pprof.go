//go:build pprof

package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	log.Println("Started pprof server.")
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}
