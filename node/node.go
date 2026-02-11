package main

import (
	"flag"
	"log"
)

var nodesToFetch = flag.Int("fetchNodes", 9, "amount of nodes to use for when fetching list of domains. higher the number means more time but more secure. 9 is recommended")

func main() {
	flag.Parse()
	if exists() {
		log.Println("update")
	} else {
		newStartup()
	}
}
