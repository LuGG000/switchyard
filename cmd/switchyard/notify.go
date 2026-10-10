package main

import (
	"context"

	"github.com/LuGG000/switchyard/internal/notify"
)

// sendNotification shows a desktop notification without holding anything up; a system
// without a way to show one just gets none.
func sendNotification(text string) {
	go func() { _ = notify.Send(context.Background(), "switchyard", text) }()
}
