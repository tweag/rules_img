package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var (
	registryPath = flag.String("registry-path", "", "Path where the local Bazel registry should be created")
	distdirPath  = flag.String("distdir-path", "", "Path where the distdir should be created")
)

func main() {
	flag.Parse()

	if *registryPath == "" {
		p, err := os.MkdirTemp("", "devmode-registry-")
		if err != nil {
			log.Fatalf("Error creating temp registry dir: %v", err)
		}
		*registryPath = p
	}
	if *distdirPath == "" {
		p, err := os.MkdirTemp("", "devmode-distdir-")
		if err != nil {
			log.Fatalf("Error creating temp distdir: %v", err)
		}
		*distdirPath = p
	}

	// Convert relative paths to absolute
	var err error
	*registryPath, err = filepath.Abs(*registryPath)
	if err != nil {
		log.Fatalf("Error resolving registry path: %v", err)
	}
	*distdirPath, err = filepath.Abs(*distdirPath)
	if err != nil {
		log.Fatalf("Error resolving distdir path: %v", err)
	}

	fmt.Printf("Building local BCR and distdir...\n")
	fmt.Printf("Registry path: %s\n", *registryPath)
	fmt.Printf("Distdir path: %s\n", *distdirPath)

	// Initial build and copy
	if err := copyAll(); err != nil {
		log.Fatalf("Initial build failed: %v", err)
	}

	fmt.Println("Use in your downstream repo:")
	fmt.Printf("  bazel build --distdir=%s --registry=%s --registry=https://bcr.bazel.build/ //your/target\n\n", *distdirPath, *registryPath)

	fmt.Println("\nOr set the following flags in the .bazelrc of your downstream repo:")
	fmt.Printf("common --distdir=%s\n", *distdirPath)
	fmt.Printf("common --registry=file://%s\n", *registryPath)
	fmt.Printf("common --registry=https://bcr.bazel.build/\n")

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Handle ibazel notifications via stdin
	go handleIbazelNotifications()

	// Wait for interrupt signal
	<-sigCh
	fmt.Println("\nShutting down...")
}

func handleIbazelNotifications() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()
		fmt.Printf("📦 ibazel notification: %s\n", command)

		// TODO: here we should update the bcr / distdir accourding to the notification.
		switch command {
		case "IBAZEL_BUILD_STARTED":
			fmt.Println("... rebuilding ...")
		case "IBAZEL_BUILD_COMPLETED SUCCESS":
			if err := copyAll(); err != nil {
				fmt.Printf("Error during rebuild: %v\n", err)
			} else {
				fmt.Println("✅ BCR and distdir updated")
			}
		case "IBAZEL_BUILD_COMPLETED FAILURE":
		default:
			fmt.Printf("Unknown ibazel command: %s\n", command)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading from stdin: %v\n", err)
	}
}

func copyAll() error {
	// Copy BCR to registry path
	if err := copyBCR(); err != nil {
		return fmt.Errorf("failed to copy BCR: %v", err)
	}

	// Copy distdir to distdir path
	if err := copyDistdir(); err != nil {
		return fmt.Errorf("failed to copy distdir: %v", err)
	}

	return nil
}

func copyBCR() error {
	// Find the BCR output directory
	localBCRSrc, err := runfiles.Rlocation("_main/img/private/release/bcr.local")
	if err != nil {
		return fmt.Errorf("failed to find local bcr: %v", err)
	}

	fmt.Printf("📂 Copying BCR from %s to %s\n", localBCRSrc, *registryPath)

	// Remove existing registry directory
	if err := os.RemoveAll(*registryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing registry: %v", err)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(*registryPath), 0755); err != nil {
		return fmt.Errorf("failed to create registry parent dir: %v", err)
	}

	// Copy the BCR directory
	cmd := exec.Command("cp", "-r", localBCRSrc, *registryPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy BCR directory: %v", err)
	}

	// Change permissions to be writable
	cmd = exec.Command("chmod", "-R", "u+w", *registryPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy BCR directory: %v", err)
	}

	return nil
}

func copyDistdir() error {
	// Find the distdir output directory
	distdirSrcPath, err := runfiles.Rlocation("_main/img/private/release/airgapped.distdir")
	if err != nil {
		return fmt.Errorf("failed to find distdir: %v", err)
	}

	fmt.Printf("📂 Copying distdir from %s to %s\n", distdirSrcPath, *distdirPath)

	// Remove existing distdir directory
	if err := os.RemoveAll(*distdirPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing distdir: %v", err)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(*distdirPath), 0755); err != nil {
		return fmt.Errorf("failed to create distdir parent dir: %v", err)
	}

	// Copy the distdir directory
	cmd := exec.Command("cp", "-r", distdirSrcPath, *distdirPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy distdir directory: %v", err)
	}

	// Change permissions to be writable
	cmd = exec.Command("chmod", "-R", "u+w", *distdirPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy BCR directory: %v", err)
	}

	return nil
}
