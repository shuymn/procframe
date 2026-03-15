package main

import (
	"context"
	"log/slog"
)

func main() {
	slog.InfoContext(context.Background(), "Hello, World!")
}
