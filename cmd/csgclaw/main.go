package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"csgclaw/cli"
)

func main() {
	log.SetFlags(0)

	app := cli.New()
	if err := app.Execute(context.Background(), os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}
}
