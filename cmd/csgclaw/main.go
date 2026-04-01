package main

import (
	"context"
	"log"
	"os"

	"csgclaw/cli"
)

func main() {
	log.SetFlags(0)

	app := cli.New()
	if err := app.Execute(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
