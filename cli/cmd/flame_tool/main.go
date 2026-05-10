package main

import (
	"context"

	"github.com/wakuwaku3/flame/cli/internal/root"
	"github.com/wakuwaku3/flame/lib/clix"
)

func main() {
	clix.Main(context.Background(), root.Execute)
}
