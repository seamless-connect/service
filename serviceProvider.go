package main

import (
	"os"

	"github.com/seamlessdns/service/serviceProvider"
)

func main() {
	os.Exit(serviceProvider.Run())
}
