package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/projectbluefin/donate-clanker/internal/app"
	"github.com/projectbluefin/donate-clanker/internal/config"
)

func main() {
	opts, err := config.Parse(os.Args[1:], environmentMap(os.Environ()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := app.Run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func environmentMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
