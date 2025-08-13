package push

import (
	"fmt"
	"os"
	"time"

	registryv1 "github.com/malt3/go-containerregistry/pkg/v1"
)

func progressPrinter(updates <-chan registryv1.Update) {
	var lastUpdate time.Time
	for update := range updates {
		relative := float64(update.Complete) / float64(update.Total) * 100
		if update.Error != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", update.Error)
			continue
		}
		if time.Since(lastUpdate) < 10*time.Millisecond {
			// Avoid printing too frequently
			continue
		}
		fmt.Fprintf(os.Stderr, "\033[KProgress: %.2f %% (%v / %v bytes)\r", relative, update.Complete, update.Total)
		lastUpdate = time.Now()
	}
	fmt.Fprintf(os.Stderr, "\033[K") // Clear the line after progress is done
}
