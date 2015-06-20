package main

import (
	"fmt"
	"os"

	"github.com/laher/wget-go/wget"
)

func main() {
	err, i := wget.WgetCli(os.Args)
	if err != nil {
		fmt.Printf("Error: %+v. %d\n", err, i)
		os.Exit(1)
	}
}
