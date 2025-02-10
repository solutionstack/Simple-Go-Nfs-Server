package main

import (
	"go-fs/cmd"
	"log"
)

func main() {

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
